package sshclient

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

// Cancellation has no network work; leave room for slow CI scheduling.
const remoteForwardRequestTestDeadline = 5 * time.Second

type waitingRemoteTransport struct {
	started   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func (transport *waitingRemoteTransport) Listen(_, _ string) (net.Listener, error) {
	close(transport.started)
	<-transport.closed
	return nil, net.ErrClosed
}

func (transport *waitingRemoteTransport) Close() error {
	transport.closeOnce.Do(func() { close(transport.closed) })
	return nil
}

func TestCancellingRemoteListenerRequestClosesTransportAndFinishesTheWait(t *testing.T) {
	transport := &waitingRemoteTransport{started: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { _ = transport.Close() })
	finished := make(chan error, 1)
	go func() { _, err := requestRemoteForward(ctx, transport, "127.0.0.1:9080"); finished <- err }()
	select {
	case <-transport.started:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("remote listener request never started")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled request = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("remote listener request survived cancellation")
	}
	select {
	case <-transport.closed:
	default:
		t.Fatal("transport was retained with an unanswered global request")
	}
}

func TestAlreadyCancelledRemoteListenerRequestDoesNotTouchTheTransport(t *testing.T) {
	transport := &waitingRemoteTransport{started: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := requestRemoteForward(ctx, transport, "127.0.0.1:9080"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request = %v", err)
	}
	select {
	case <-transport.started:
		t.Fatal("cancelled request started a listener")
	case <-transport.closed:
		t.Fatal("cancelled request closed an unused transport")
	default:
	}
}

type waitingCloseConnection struct {
	net.Conn
	started chan struct{}
	closed  <-chan struct{}
}

func (connection *waitingCloseConnection) Close() error {
	close(connection.started)
	<-connection.closed
	return nil
}

func TestCancellingRemoteForwardStopFinishesEvenWhenATunnelCannotClose(t *testing.T) {
	transport := &waitingRemoteTransport{started: make(chan struct{}), closed: make(chan struct{})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close(); _ = transport.Close() })
	forward := newRemoteForward(listener, "127.0.0.1:3000", transport)
	connection := &waitingCloseConnection{started: make(chan struct{}), closed: transport.closed}
	forward.connections[connection] = struct{}{}
	go forward.accept()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	finished := make(chan error, 1)
	go func() { finished <- forward.close(ctx) }()
	select {
	case <-connection.started:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("remote tunnel never started closing")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled stop = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("remote tunnel survived stop cancellation")
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("listener after cancellation = %v", err)
	}
}

type lateRemoteListenerTransport struct {
	started   chan struct{}
	reply     chan net.Listener
	closed    chan struct{}
	closeOnce sync.Once
}

func (transport *lateRemoteListenerTransport) Listen(_, _ string) (net.Listener, error) {
	close(transport.started)
	return <-transport.reply, nil
}

func (transport *lateRemoteListenerTransport) Close() error {
	transport.closeOnce.Do(func() { close(transport.closed) })
	return nil
}

type observedRemoteListener struct {
	net.Listener
	closed    chan struct{}
	closeOnce sync.Once
}

func (listener *observedRemoteListener) Close() error {
	err := listener.Listener.Close()
	listener.closeOnce.Do(func() { close(listener.closed) })
	return err
}

func TestCancelledListenReturnsBeforeALateReplyAndClosesItsListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	observed := &observedRemoteListener{Listener: listener, closed: make(chan struct{})}
	transport := &lateRemoteListenerTransport{started: make(chan struct{}), reply: make(chan net.Listener, 1), closed: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { transport.reply <- observed }) }
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	returned := make(chan error, 1)
	go func() { _, err := requestRemoteForward(ctx, transport, "127.0.0.1:9080"); returned <- err }()
	select {
	case <-transport.started:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("listen never started")
	}
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled request = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("cancelled request waited for its late reply")
	}
	release()
	select {
	case <-observed.closed:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("late reply left its unused listener open")
	}
}
