package terminal_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
)

// Allow slow CI scheduling while still catching a session lock held across I/O.
const forwardReplyTestDeadline = 5 * time.Second

type waitingForwardProcess struct {
	*fakeProcess
	operation   string
	requested   chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	stopped     chan string
}

func (process *waitingForwardProcess) StartForward(_, _, _ string) (terminal.Forward, error) {
	if process.operation == "start" {
		close(process.requested)
		<-process.release
	}
	return terminal.Forward{ID: "old-forward"}, nil
}

func (process *waitingForwardProcess) StopForward(id string) error {
	if process.operation == "stop" {
		close(process.requested)
		<-process.release
	}
	process.stopped <- id
	return nil
}

func (process *waitingForwardProcess) allowReply() {
	process.releaseOnce.Do(func() { close(process.release) })
}

func TestPendingForwardRequestsLeaveSessionViewsAndClosingAvailable(t *testing.T) {
	for _, operation := range []string{"start", "stop"} {
		t.Run(operation, func(t *testing.T) {
			process := &waitingForwardProcess{fakeProcess: newFakeProcess(), operation: operation, requested: make(chan struct{}), release: make(chan struct{}), stopped: make(chan string, 1)}
			registry := &terminal.Registry{Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 4, Scrollback: 1024} }}
			session, err := registry.Open(context.Background(), terminal.Spec{Kind: terminal.KindSSH, Alias: "server", Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { process.allowReply(); _ = registry.Close(session.ID()) })
			finished := make(chan error, 1)
			go func() {
				if operation == "start" {
					_, err := session.StartForward("remote", "9080", "127.0.0.1:3000")
					finished <- err
				} else {
					finished <- session.StopForward("existing-forward")
				}
			}()
			select {
			case <-process.requested:
			case <-time.After(forwardReplyTestDeadline):
				t.Fatal("forward request never started")
			}
			viewed := make(chan struct{})
			go func() { _ = session.View(); close(viewed) }()
			select {
			case <-viewed:
			case <-time.After(forwardReplyTestDeadline):
				t.Fatal("a forwarding reply blocked the session view")
			}
			closed := make(chan error, 1)
			go func() { closed <- registry.Close(session.ID()) }()
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(forwardReplyTestDeadline):
				t.Fatal("a forwarding reply blocked closing the session")
			}
			process.allowReply()
			select {
			case err := <-finished:
				if !errors.Is(err, terminal.ErrNotConnected) {
					t.Fatalf("forwarding on a closed generation = %v", err)
				}
			case <-time.After(forwardReplyTestDeadline):
				t.Fatal("forward request survived the discarded generation")
			}
			if operation == "start" {
				select {
				case id := <-process.stopped:
					if id != "old-forward" {
						t.Fatalf("discarded generation listener = %q", id)
					}
				case <-time.After(forwardReplyTestDeadline):
					t.Fatal("discarded generation retained its listener")
				}
			}
		})
	}
}
