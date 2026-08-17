package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// fakeProbe は、生きているエンジンの答えを台本どおりに返す。
//
// **答えは呼ばれるたびに変わってよい。** 解錠は待っているあいだに起きるもので、
// 一度きりの応答では待ちそのものを検査できない。
type fakeProbe struct {
	mutex       sync.Mutex
	answers     []statusAnswer
	statusCalls int
	statusErr   error
	connections []string
	connection  connectAnswer
}

func (probe *fakeProbe) Status(context.Context) (statusAnswer, error) {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	probe.statusCalls++
	if probe.statusErr != nil {
		return statusAnswer{}, probe.statusErr
	}
	if len(probe.answers) == 0 {
		return statusAnswer{}, errors.New("no scripted answer")
	}
	answer := probe.answers[0]
	if len(probe.answers) > 1 {
		probe.answers = probe.answers[1:]
	}
	return answer, nil
}

func (probe *fakeProbe) Connection(_ context.Context, alias string) (connectAnswer, error) {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	probe.connections = append(probe.connections, alias)
	return probe.connection, nil
}

func (probe *fakeProbe) requestedAliases() []string {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	return append([]string(nil), probe.connections...)
}

func unlockedDesktop() statusAnswer {
	return statusAnswer{Owner: handoff.OwnerDesktop, ProtocolVersion: handoff.ProtocolVersion, Vault: true, Unlocked: true}
}

func lockedDesktop() statusAnswer {
	return statusAnswer{Owner: handoff.OwnerDesktop, ProtocolVersion: handoff.ProtocolVersion, Vault: true}
}

func withOwner(answer statusAnswer, owner handoff.Owner) statusAnswer {
	answer.Owner = owner
	return answer
}

// stateWithEngine は、handoff だけを置く。応答するのは fakeProbe である。
func stateWithEngine(t *testing.T, owner handoff.Owner) string {
	t.Helper()
	stateDir := t.TempDir()
	document := testHandoff("http://127.0.0.1:1")
	document.Owner = owner
	if err := handoff.Write(stateDir, document); err != nil {
		t.Fatal(err)
	}
	return stateDir
}

func reach(
	t *testing.T, stateDir string, launcher desktopLauncher, probe engineProbe, stderr *bytes.Buffer,
) (engineProbe, error) {
	t.Helper()
	return reachWaiting(t, stateDir, launcher, probe, stderr, true)
}

func reachWaiting(
	t *testing.T, stateDir string, launcher desktopLauncher, probe engineProbe,
	stderr *bytes.Buffer, wait bool,
) (engineProbe, error) {
	t.Helper()
	return reachUnlockedEngine(context.Background(), stateDir, &http.Client{}, launcher,
		func(handoff.Handoff) engineProbe { return probe }, stderr, wait)
}

// 設計 8.2 の六つの経路。**保管庫の不在は解錠ではない**ので、そこも同じ表で見る。
func TestReachUnlockedEngineFollowsTheOwner(t *testing.T) {
	for _, test := range []struct {
		name string
		// engine が居るなら handoff を置き、probe が答える。
		owner     handoff.Owner
		running   bool
		answers   []statusAnswer
		available bool
		wantErr   string
		// 窓を前へ出した回数。待たない経路では 0 でなければならない。
		wantLaunches int
	}{
		{
			name:  "desktop unlocked connects without focusing the window",
			owner: handoff.OwnerDesktop, running: true, available: true,
			answers: []statusAnswer{unlockedDesktop()}, wantLaunches: 0,
		},
		{
			name:  "headless unlocked connects immediately",
			owner: handoff.OwnerHeadless, running: true, available: true,
			answers: []statusAnswer{withOwner(unlockedDesktop(), handoff.OwnerHeadless)}, wantLaunches: 0,
		},
		{
			name:  "headless locked does not wait on an invisible window",
			owner: handoff.OwnerHeadless, running: true, available: true,
			answers: []statusAnswer{withOwner(lockedDesktop(), handoff.OwnerHeadless)},
			wantErr: "sshc vault unlock", wantLaunches: 0,
		},
		{
			name:  "headless without a vault says how to create one",
			owner: handoff.OwnerHeadless, running: true, available: true,
			answers: []statusAnswer{{Owner: handoff.OwnerHeadless, ProtocolVersion: handoff.ProtocolVersion}},
			wantErr: "sshc vault create", wantLaunches: 0,
		},
		{
			name:    "no engine and no display sends the user to headless",
			running: false, available: false,
			wantErr: "sshc headless", wantLaunches: 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			if test.running {
				stateDir = stateWithEngine(t, test.owner)
			}
			launcher := &countingLauncher{available: test.available}
			probe := &fakeProbe{answers: test.answers}
			var stderr bytes.Buffer

			session, err := reach(t, stateDir, launcher, probe, &stderr)

			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want an unlocked engine", err)
				}
				if session == nil {
					t.Error("session carries no probe")
				}
			} else {
				if err == nil {
					t.Fatalf("err = nil, want %q", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("err = %q, want it to name %q", err, test.wantErr)
				}
			}
			if launcher.launches != test.wantLaunches {
				t.Errorf("launches = %d, want %d", launcher.launches, test.wantLaunches)
			}
		})
	}
}

// 施錠された desktop は、窓を **一度だけ** 前へ出し、あとは待つ。poll ごとに
// 起こせば、待っているあいだじゅう画面を奪い続けることになる。
func TestReachUnlockedEngineFocusesOnceThenWaits(t *testing.T) {
	stateDir := stateWithEngine(t, handoff.OwnerDesktop)
	launcher := &countingLauncher{available: true}
	probe := &fakeProbe{answers: []statusAnswer{
		lockedDesktop(), lockedDesktop(), lockedDesktop(), unlockedDesktop(),
	}}
	var stderr bytes.Buffer

	session, err := reach(t, stateDir, launcher, probe, &stderr)

	if err != nil {
		t.Fatalf("err = %v, want the wait to end in an unlocked engine", err)
	}
	if launcher.launches != 1 {
		t.Errorf("launches = %d, want exactly 1", launcher.launches)
	}
	if probe.statusCalls < 3 {
		t.Errorf("status calls = %d, want the wait to have polled", probe.statusCalls)
	}
	if session == nil {
		t.Error("session carries no probe")
	}
	if !strings.Contains(stderr.String(), "locked") {
		t.Errorf("stderr = %q, want the user told why they are waiting", stderr.String())
	}
}

// 保管庫が無い desktop も待つ。作るのは窓でも `sshc vault create` でもよく、
// どちらも同じエンジンを変えるので、待ち手はどちらが起きたかを知らなくてよい。
func TestReachUnlockedEngineWaitsForAVaultToBeCreated(t *testing.T) {
	stateDir := stateWithEngine(t, handoff.OwnerDesktop)
	launcher := &countingLauncher{available: true}
	probe := &fakeProbe{answers: []statusAnswer{
		{Owner: handoff.OwnerDesktop, ProtocolVersion: handoff.ProtocolVersion},
		unlockedDesktop(),
	}}
	var stderr bytes.Buffer

	if _, err := reach(t, stateDir, launcher, probe, &stderr); err != nil {
		t.Fatalf("err = %v, want the wait to end once the vault exists and is open", err)
	}
	if launcher.launches != 1 {
		t.Errorf("launches = %d, want exactly 1", launcher.launches)
	}
	if !strings.Contains(stderr.String(), "vault create") {
		t.Errorf("stderr = %q, want the create command named", stderr.String())
	}
}

// engine が居ないが画面はある。外殻を起こし、その handoff が出るまで待って、
// desktop の経路へ合流する。
func TestReachUnlockedEngineLaunchesAndWaitsForItsHandoff(t *testing.T) {
	stateDir := t.TempDir()
	probe := &fakeProbe{answers: []statusAnswer{unlockedDesktop()}}
	launcher := &writingLauncher{stateDir: stateDir, t: t}

	session, err := reach(t, stateDir, launcher, probe, &bytes.Buffer{})

	if err != nil {
		t.Fatalf("err = %v, want the launched engine to be used", err)
	}
	if session == nil {
		t.Error("session carries no probe")
	}
	if launcher.launches.Load() != 1 {
		t.Errorf("launches = %d, want exactly 1", launcher.launches.Load())
	}
}

// **待っていた一台が入れ替わったら降りる。** 別のエンジンが解錠されても、それは
// この待ち手が待っていたものではない。黙って乗り換えれば、利用者が起こしたので
// はないエンジンが接続材料を渡すことになる。
func TestWaitForDesktopUnlockRefusesADifferentEngine(t *testing.T) {
	initial := testHandoff("http://127.0.0.1:1")
	initial.Owner = handoff.OwnerDesktop
	for _, test := range []struct {
		name   string
		answer statusAnswer
	}{
		{name: "the owner changed", answer: withOwner(unlockedDesktop(), handoff.OwnerHeadless)},
		{name: "the protocol changed", answer: statusAnswer{
			Owner: handoff.OwnerDesktop, ProtocolVersion: handoff.ProtocolVersion + 1, Vault: true, Unlocked: true,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &fakeProbe{answers: []statusAnswer{test.answer}}
			err := waitForDesktopUnlock(context.Background(), initial, probe, time.Millisecond)
			if !errors.Is(err, errEngineChanged) {
				t.Fatalf("err = %v, want errEngineChanged", err)
			}
		})
	}
}

func TestWaitForDesktopUnlockStopsWhenTheEngineExits(t *testing.T) {
	initial := testHandoff("http://127.0.0.1:1")
	initial.Owner = handoff.OwnerDesktop
	probe := &fakeProbe{statusErr: errors.New("connection refused")}

	err := waitForDesktopUnlock(context.Background(), initial, probe, time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("err = %v, want the engine's exit reported", err)
	}
}

func TestWaitForDesktopUnlockEndsOnInterrupt(t *testing.T) {
	initial := testHandoff("http://127.0.0.1:1")
	initial.Owner = handoff.OwnerDesktop
	probe := &fakeProbe{answers: []statusAnswer{lockedDesktop()}}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- waitForDesktopUnlock(ctx, initial, probe, time.Millisecond) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, errInterrupted) {
			t.Fatalf("err = %v, want errInterrupted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the wait ignored the cancelled context")
	}
}

// 解錠を待ったあと、利用者に打ち直させない。元の接続先をそのまま一回要求する。
func TestConnectionIsRequestedOnceForTheOriginalAlias(t *testing.T) {
	stateDir := stateWithEngine(t, handoff.OwnerDesktop)
	launcher := &countingLauncher{available: true}
	probe := &fakeProbe{answers: []statusAnswer{lockedDesktop(), unlockedDesktop()}}

	session, err := reach(t, stateDir, launcher, probe, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("reach: %v", err)
	}
	if _, err := session.Connection(context.Background(), "the-original-alias"); err != nil {
		t.Fatalf("connection: %v", err)
	}

	requested := probe.requestedAliases()
	if len(requested) != 1 || requested[0] != "the-original-alias" {
		t.Errorf("requested aliases = %v, want the original alias exactly once", requested)
	}
}

// writingLauncher は、起こされたときに handoff を書く外殻を演じる。
type writingLauncher struct {
	stateDir string
	t        *testing.T
	launches atomic.Int32
}

func (launcher *writingLauncher) Available() (bool, error) { return true, nil }

func (launcher *writingLauncher) Launch(context.Context) error {
	launcher.launches.Add(1)
	document := testHandoff("http://127.0.0.1:1")
	document.Owner = handoff.OwnerDesktop
	if err := handoff.Write(launcher.stateDir, document); err != nil {
		launcher.t.Errorf("write handoff: %v", err)
	}
	return nil
}
