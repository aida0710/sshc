package remoteengine

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallersArrivingDuringAnAttemptShareItsResult(t *testing.T) {
	var attempts sharedAttempt[int]
	var started atomic.Int32
	release := make(chan struct{})
	failure := errors.New("the host did not answer")
	start := func(context.Context) (int, error) {
		started.Add(1)
		<-release
		return 0, failure
	}
	var waiting sync.WaitGroup
	results := make(chan error, 3)
	for range 3 {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			_, err := attempts.do(context.Background(), time.Minute, start)
			results <- err
		}()
	}
	for started.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	// The attempt blocks until released, so callers that reach do in this time join it.
	time.Sleep(100 * time.Millisecond)
	close(release)
	waiting.Wait()
	close(results)
	for err := range results {
		if !errors.Is(err, failure) {
			t.Fatalf("a caller got %v, want the shared failure", err)
		}
	}
	if started.Load() != 1 {
		t.Fatalf("started %d attempts for callers that arrived together, want 1", started.Load())
	}
}

func TestACallerThatGivesUpDoesNotCancelTheAttemptOthersWaitFor(t *testing.T) {
	var attempts sharedAttempt[string]
	var started atomic.Int32
	release := make(chan struct{})
	start := func(ctx context.Context) (string, error) {
		started.Add(1)
		select {
		case <-release:
			return "link", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	impatient, giveUp := context.WithCancel(context.Background())
	gaveUp := make(chan error, 1)
	go func() {
		_, err := attempts.do(impatient, time.Minute, start)
		gaveUp <- err
	}()
	time.Sleep(10 * time.Millisecond)
	patient := make(chan string, 1)
	go func() {
		value, _ := attempts.do(context.Background(), time.Minute, start)
		patient <- value
	}()
	giveUp()
	if err := <-gaveUp; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that gave up got %v", err)
	}
	attempts.mutex.Lock()
	running := attempts.running != nil
	attempts.mutex.Unlock()
	if !running {
		t.Fatal("the attempt ended when the caller that started it gave up")
	}
	// The attempt blocks until released, so the patient caller reaches do and
	// joins it in this time rather than starting its own after it ends.
	time.Sleep(100 * time.Millisecond)
	close(release)
	if value := <-patient; value != "link" {
		t.Fatalf("the remaining caller got %q, want the attempt's result", value)
	}
	if started.Load() != 1 {
		t.Fatalf("started %d attempts, want the one the impatient caller began to carry on", started.Load())
	}
}

func TestALinkFinishedAfterCloseIsClosedRatherThanKept(t *testing.T) {
	release := make(chan struct{})
	link := newFakeLink("", 0)
	host := newHostLink(func(context.Context) (Link, error) {
		<-release
		return link, nil
	}, time.Minute)
	result := make(chan error, 1)
	go func() {
		_, err := host.live(context.Background())
		result <- err
	}()
	time.Sleep(10 * time.Millisecond)
	host.close()
	close(release)
	if err := <-result; !errors.Is(err, ErrClosed) {
		t.Fatalf("live after close = %v, want ErrClosed", err)
	}
	select {
	case <-link.done:
	default:
		t.Fatal("the link that finished after close was left open")
	}
}

func TestASilentSSHConnectionIsGivenUpOnAndTheEngineReachedAgainOverANewOne(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	var connections atomic.Int32
	var current *fakeLink
	manager := NewManager(ManagerOptions{
		Store:        newStore(t),
		LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			connections.Add(1)
			current = newFakeLink(remote.server.URL+"/#x", 0)
			return &handoffLink{fakeLink: current, handoff: remote.handoffDocument("v9")}, nil
		},
	})
	t.Cleanup(manager.Close)
	manager.answerLimit = 100 * time.Millisecond
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	remote.stalled.Store(true)
	current.silent.Store(true)
	started := time.Now()
	status, body, _ := forward(t, manager, http.MethodGet, "")
	if status != http.StatusBadGateway || !strings.Contains(body, "remote_engine_unreachable") {
		t.Fatalf("a request the remote never answers = %d %s", status, body)
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("waited %v for an answer that never came", waited)
	}
	remote.stalled.Store(false)
	if status, body, _ := forward(t, manager, http.MethodGet, ""); status != http.StatusOK {
		t.Fatalf("the next request = %d %s", status, body)
	}
	if connections.Load() != 2 {
		t.Fatalf("opened %d SSH connections, want a new one after the silent one was given up", connections.Load())
	}
}

func TestAnAttemptThatNeverEndsIsCancelledAtItsLimitAndTheNextCallerStartsAnew(t *testing.T) {
	var attempts sharedAttempt[string]
	var started atomic.Int32
	start := func(ctx context.Context) (string, error) {
		started.Add(1)
		<-ctx.Done()
		return "", ctx.Err()
	}
	if _, err := attempts.do(context.Background(), 50*time.Millisecond, start); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("an attempt that never ends = %v, want its limit", err)
	}
	if _, err := attempts.do(context.Background(), 50*time.Millisecond, start); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the next attempt = %v, want its limit", err)
	}
	if started.Load() != 2 {
		t.Fatalf("started %d attempts, want a new one after the first reached its limit", started.Load())
	}
}

func TestAConnectionThatNeverAuthenticatesEndsAtTheLinkLimit(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Store: newStore(t), LocalVersion: "v9",
		Connect: func(ctx context.Context, _ string) (Link, error) {
			// sshclient stops its ConnectTimeout once authentication begins, so
			// only the attempt's own limit ends a server that never answers.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	t.Cleanup(manager.Close)
	manager.linkLimit = 100 * time.Millisecond
	if err := manager.options.Store.AddEngine("r540"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := manager.EngineStatus(ctx, "r540")
		callerExpired := ctx.Err() != nil
		cancel()
		if err == nil || callerExpired {
			t.Fatalf("status #%d = %v, want the connection attempt to end on its own", attempt, err)
		}
	}
}

func TestClosingTheManagerCancelsAConnectionAttemptInProgress(t *testing.T) {
	cancelled := make(chan struct{})
	manager := NewManager(ManagerOptions{
		Store: newStore(t), LocalVersion: "v9",
		Connect: func(ctx context.Context, _ string) (Link, error) {
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		},
	})
	if err := manager.options.Store.AddEngine("r540"); err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = manager.EngineStatus(context.Background(), "r540") }()
	time.Sleep(50 * time.Millisecond)
	manager.Close()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection attempt kept running after the manager closed")
	}
}

func TestAnEngineThatNeverAnswersTheSessionRequestDoesNotHoldLaterRequests(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	remote.sessionStalled.Store(true)
	manager := NewManager(ManagerOptions{
		Store: newStore(t), LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			return &handoffLink{fakeLink: newFakeLink(remote.server.URL+"/#x", 0), handoff: remote.handoffDocument("v9")}, nil
		},
	})
	t.Cleanup(manager.Close)
	manager.sessionLimit = 100 * time.Millisecond
	if err := manager.options.Store.AddEngine("r540"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := manager.EngineStatus(ctx, "r540"); err == nil || ctx.Err() != nil {
		t.Fatalf("status from an engine that never answers /cli/session = %v, want the attach to end on its own", err)
	}
	remote.sessionStalled.Store(false)
	if _, err := manager.EngineStatus(ctx, "r540"); err != nil {
		t.Fatalf("status after the engine answers again = %v", err)
	}
}

func TestAStalledEngineOverAnAnsweringSSHConnectionKeepsTheConnection(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	var connections atomic.Int32
	var current *fakeLink
	manager := NewManager(ManagerOptions{
		Store: newStore(t), LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			connections.Add(1)
			current = newFakeLink(remote.server.URL+"/#x", 0)
			return &handoffLink{fakeLink: current, handoff: remote.handoffDocument("v9")}, nil
		},
	})
	t.Cleanup(manager.Close)
	manager.answerLimit = 100 * time.Millisecond
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	remote.stalled.Store(true)
	if status, body, _ := forward(t, manager, http.MethodGet, ""); status != http.StatusBadGateway {
		t.Fatalf("a request the remote never answers = %d %s", status, body)
	}
	// 同じ SSH の接続の上で、エンジンの更新の出力や別タブの中継が続いている。
	select {
	case <-current.done:
		t.Fatal("the SSH connection was closed although it still answers keepalives")
	default:
	}
	remote.stalled.Store(false)
	if status, body, _ := forward(t, manager, http.MethodGet, ""); status != http.StatusOK {
		t.Fatalf("the next request = %d %s", status, body)
	}
	if connections.Load() != 1 {
		t.Fatalf("opened %d SSH connections, want the answering one kept", connections.Load())
	}
}
