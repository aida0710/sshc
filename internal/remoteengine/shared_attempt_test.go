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
			_, err := attempts.do(context.Background(), start)
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
		_, err := attempts.do(impatient, start)
		gaveUp <- err
	}()
	time.Sleep(10 * time.Millisecond)
	patient := make(chan string, 1)
	go func() {
		value, _ := attempts.do(context.Background(), start)
		patient <- value
	}()
	giveUp()
	if err := <-gaveUp; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that gave up got %v", err)
	}
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
	})
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

func TestARemoteEngineThatStopsAnsweringIsGivenUpOnAndReachedAgainOverANewConnection(t *testing.T) {
	remote := newFakeRemoteEngine(t)
	var connections atomic.Int32
	manager := NewManager(ManagerOptions{
		Store:        newStore(t),
		LocalVersion: "v9",
		Connect: func(context.Context, string) (Link, error) {
			connections.Add(1)
			return &handoffLink{fakeLink: newFakeLink(remote.server.URL+"/#x", 0), handoff: remote.handoffDocument("v9")}, nil
		},
	})
	t.Cleanup(manager.Close)
	manager.answerLimit = 100 * time.Millisecond
	if err := manager.AddEngine(context.Background(), "r540"); err != nil {
		t.Fatal(err)
	}
	remote.stalled.Store(true)
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
