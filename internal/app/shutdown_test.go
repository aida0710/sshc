package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/terminal"
)

type stuckProcess struct {
	release chan struct{}
	forced  atomic.Bool
	once    sync.Once
}

func newStuckProcess() *stuckProcess { return &stuckProcess{release: make(chan struct{})} }

func (p *stuckProcess) Read([]byte) (int, error) {
	<-p.release
	return 0, errors.New("closed")
}
func (p *stuckProcess) Write(b []byte) (int, error) { return len(b), nil }
func (p *stuckProcess) Resize(terminal.Size) error  { return nil }

func (p *stuckProcess) Hangup() error { <-p.release; return nil }

func (p *stuckProcess) Wait() terminal.ExitInfo { <-p.release; return terminal.ExitInfo{Code: -1} }
func (p *stuckProcess) Close() error            { return nil }

func (p *stuckProcess) ForceClose() error {
	p.forced.Store(true)
	p.once.Do(func() { close(p.release) })
	return nil
}

func testDependencies(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{
		Home:            t.TempDir(),
		Random:          bytes.NewReader(bytes.Repeat([]byte{0x31}, 512)),
		Listen:          net.Listen,
		UI:              fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:           handoff.OwnerEngine,
		PID:             4242,
		ShutdownTimeout: 40 * time.Millisecond,
	}
}

func TestUnwindForcesABlockedTerminalAtTheDeadline(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()

	process := newStuckProcess()
	if _, err := built.terminals.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "unreachable",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	}); err != nil {
		t.Fatal(err)
	}

	unwound := make(chan error, 1)
	started := time.Now()
	go func() { unwound <- built.unwind(dependencies) }()

	select {
	case err := <-unwound:
		if err != nil {
			t.Fatalf("unwind = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("unwind never returned; the deadline did not force the blocked terminal")
	}
	if !process.forced.Load() {
		t.Fatal("the blocked terminal was never forced")
	}
	if elapsed := time.Since(started); elapsed < dependencies.ShutdownTimeout {
		t.Fatalf("unwind returned in %v, before its own %v deadline", elapsed, dependencies.ShutdownTimeout)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestUnwindContinuesAfterAFailedHandoffRemoval(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()

	if err := handoff.Remove(HandoffDir(dependencies.Home), built.document.Secret); err != nil {
		t.Fatal(err)
	}

	if err := built.unwind(dependencies); err != nil {
		t.Fatalf("unwind = %v", err)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestUnwindCancelsAndJoinsAutoSyncBeforeReturning(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()

	autoContext, cancel := context.WithCancel(context.Background())
	built.autoCancel = cancel
	built.autoDone = make(chan struct{})
	release := make(chan struct{})
	go func() {
		<-autoContext.Done()
		<-release
		close(built.autoDone)
	}()

	unwound := make(chan error, 1)
	go func() { unwound <- built.unwind(dependencies) }()
	select {
	case err := <-unwound:
		t.Fatalf("unwind returned before AutoSync joined: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-unwound; err != nil {
		t.Fatalf("unwind = %v", err)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

func TestStoppingRefusesValidMutationsAfterSecurityHasAcceptedThem(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()
	t.Cleanup(func() {
		built.server.BeginShutdown()
		_ = built.server.Wait()
		<-served
	})

	base := built.server.URL()
	client := &http.Client{Timeout: 5 * time.Second}
	post := func(t *testing.T, valid bool) int {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, base+"/api/v1/session/bootstrap", nil)
		if err != nil {
			t.Fatal(err)
		}
		if valid {
			request.Header.Set("Origin", base)
			request.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	if status := post(t, true); status == http.StatusServiceUnavailable {
		t.Fatal("a mutation was refused as stopping before stopping began")
	}

	built.server.BeginStopping()

	if status := post(t, true); status != http.StatusServiceUnavailable {
		t.Fatalf("valid mutation during stopping = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if status := post(t, false); status != http.StatusForbidden {
		t.Fatalf("invalid-origin mutation during stopping = %d, want %d", status, http.StatusForbidden)
	}

	health, err := http.NewRequest(http.MethodGet, base+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	health.Header.Set("Origin", base)
	health.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := client.Do(health)
	if err != nil {
		t.Fatal(err)
	}
	status := response.StatusCode
	response.Body.Close()
	if status == http.StatusServiceUnavailable {
		t.Fatal("a read-only request was refused by the stopping gate")
	}
}
