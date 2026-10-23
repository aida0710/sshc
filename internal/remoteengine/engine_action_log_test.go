package remoteengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/sshclient"
	"sshc/internal/testwait"
)

func TestTheOutputOfARunningActionCanBeReadBeforeItEnds(t *testing.T) {
	release := make(chan struct{})
	link := &controlLink{run: func(_ context.Context, request RunRequest) (RunResult, error) {
		_, _ = request.Observe.Write([]byte("sshc: updating v1 to v2\n"))
		<-release
		_, _ = request.Observe.Write([]byte("sshc: updated to v2\n"))
		return RunResult{ExitCode: 0}, nil
	}}
	manager := newControlManager(t, newFakeRemoteEngine(t), link)
	generation, err := manager.StartAction(context.Background(), "r540", ActionUpdate)
	if err != nil {
		t.Fatal(err)
	}

	var first ActionLogRead
	waitForOutput(t, manager, &first)
	if first.Done || first.Generation != generation || first.Output != "sshc: updating v1 to v2\n" {
		t.Fatalf("while running = %+v", first)
	}
	close(release)
	last := waitForAction(t, manager)
	rest, err := manager.ActionLog("r540", first.Read.Next, actionOutputCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if !last.Succeeded || rest.Output != "sshc: updated to v2\n" {
		t.Fatalf("after the first read = %+v, rest %q", last, rest.Output)
	}
}

// waitForOutput は、r540 の操作の出力が届くまで読む。
func waitForOutput(t *testing.T, manager *Manager, read *ActionLogRead) {
	t.Helper()
	testwait.Until(t, func() bool {
		var err error
		*read, err = manager.ActionLog("r540", 0, actionOutputCapacity)
		return err == nil && read.Output != ""
	})
}

func TestASecondActionIsRefusedWhileOneRunsAndTheNextStartsAFreshLog(t *testing.T) {
	release := make(chan struct{})
	runs := 0
	link := &controlLink{run: func(_ context.Context, request RunRequest) (RunResult, error) {
		runs++
		if runs == 1 {
			_, _ = request.Observe.Write([]byte("first\n"))
			<-release
		} else {
			_, _ = request.Observe.Write([]byte("second\n"))
		}
		return RunResult{}, nil
	}}
	manager := newControlManager(t, newFakeRemoteEngine(t), link)
	first, err := manager.StartAction(context.Background(), "r540", ActionUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartAction(context.Background(), "r540", ActionRestart); !errors.Is(err, ErrActionRunning) {
		t.Fatalf("second action while the first runs = %v", err)
	}
	close(release)
	waitForAction(t, manager)

	second, err := manager.StartAction(context.Background(), "r540", ActionRestart)
	if err != nil {
		t.Fatal(err)
	}
	read := waitForAction(t, manager)
	if second != first+1 || read.Generation != second || read.Action != ActionRestart {
		t.Fatalf("generations %d then %d, read %+v", first, second, read)
	}
	if read.Read.Start != 0 || read.Output != "second\n" {
		t.Fatalf("the second action's output = %q from %d", read.Output, read.Read.Start)
	}
}

func TestAnActionThatDidNotRunToTheEndSaysWhy(t *testing.T) {
	for _, test := range []struct {
		name        string
		result      RunResult
		err         error
		wantFailure error
	}{
		{name: "no sshc on the host", result: RunResult{ExitCode: notInstalledExit}, wantFailure: ErrNotInstalled},
		{name: "past the limit", err: sshclient.ErrCommandTimedOut, wantFailure: ErrActionTimedOut},
		{name: "the connection broke", err: errors.New("ssh: connection lost")},
		{name: "sshc failed", result: RunResult{Stderr: []byte("sshc: update failed\n"), ExitCode: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			link := &controlLink{run: func(_ context.Context, request RunRequest) (RunResult, error) {
				_, _ = request.Observe.Write(test.result.Stderr)
				return test.result, test.err
			}}
			manager := newControlManager(t, newFakeRemoteEngine(t), link)
			if _, err := manager.StartAction(context.Background(), "r540", ActionUpdate); err != nil {
				t.Fatal(err)
			}
			read := waitForAction(t, manager)
			want := test.wantFailure
			if want == nil {
				want = test.err
			}
			if read.Succeeded || !errors.Is(read.Failure, want) || (want == nil && read.Failure != nil) {
				t.Fatalf("read = %+v, want failure %v", read, want)
			}
			if read.Output != string(test.result.Stderr) {
				t.Fatalf("output = %q", read.Output)
			}
		})
	}
}

func TestTheActionLogRefusesACursorPastItsEndAndKeepsOnlyTheNewestOutput(t *testing.T) {
	log := &actionLog{}
	if _, err := log.read(0, actionOutputCapacity); !errors.Is(err, ErrNoAction) {
		t.Fatalf("before any action = %v", err)
	}
	if _, err := log.begin(ActionUpdate); err != nil {
		t.Fatal(err)
	}
	overflow := strings.Repeat("a", actionOutputCapacity) + "newest\n"
	_, _ = log.Write([]byte(overflow))
	read, err := log.read(0, actionOutputCapacity)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Read.Truncated || read.Read.Start != uint64(len(overflow)-actionOutputCapacity) || !strings.HasSuffix(read.Output, "newest\n") {
		t.Fatalf("read from 0 after overflowing = start %d, truncated %t", read.Read.Start, read.Read.Truncated)
	}
	if _, err := log.read(uint64(len(overflow))+1, actionOutputCapacity); !errors.Is(err, ErrActionCursorAhead) {
		t.Fatalf("cursor past the end = %v", err)
	}
}

func TestAnActionNeedsAnAddedEngine(t *testing.T) {
	manager := newControlManager(t, newFakeRemoteEngine(t), &controlLink{run: writeResult(RunResult{})})
	if _, err := manager.StartAction(context.Background(), "elsewhere", ActionUpdate); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("start on an engine not added = %v", err)
	}
	if _, err := manager.ActionLog("elsewhere", 0, actionOutputCapacity); !errors.Is(err, ErrNotAdded) {
		t.Fatalf("log of an engine not added = %v", err)
	}
	if _, err := manager.ActionLog("r540", 0, actionOutputCapacity); !errors.Is(err, ErrNoAction) {
		t.Fatalf("log before any action = %v", err)
	}
}

// 操作は要求ではなく Manager の寿命で走る。閉じれば止まり、Close はそれが終わるまで待つ。
func TestClosingTheManagerStopsARunningAction(t *testing.T) {
	stopped := make(chan error, 1)
	link := &controlLink{run: func(ctx context.Context, _ RunRequest) (RunResult, error) {
		<-ctx.Done()
		stopped <- ctx.Err()
		return RunResult{}, ctx.Err()
	}}
	manager := newControlManager(t, newFakeRemoteEngine(t), link)
	requestContext, endRequest := context.WithCancel(context.Background())
	if _, err := manager.StartAction(requestContext, "r540", ActionUpdate); err != nil {
		t.Fatal(err)
	}
	// 要求が終わっても、操作は止まらない。
	endRequest()
	select {
	case err := <-stopped:
		t.Fatalf("the action stopped with the request: %v", err)
	default:
	}
	manager.Close()
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("the action stopped with %v", err)
	}
}
