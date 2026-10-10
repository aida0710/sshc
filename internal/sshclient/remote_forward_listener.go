package sshclient

import (
	"context"
	"net"
	"runtime"
)

type remoteListenerClose struct {
	completed chan struct{}
	err       error
}

// x/crypto/ssh sends pending forwarded channels while holding its listener
// registry mutex. If Accept exits on a transport error, Close cannot remove
// that listener until somebody consumes the remaining channels.
func startRemoteListenerClose(ctx context.Context, transport remoteForwardTransport, listener net.Listener) *remoteListenerClose {
	closing := &remoteListenerClose{completed: make(chan struct{})}
	listenerClosed := make(chan struct{})
	go func() {
		closing.err = listener.Close()
		close(listenerClosed)
	}()
	go func() {
		defer close(closing.completed)
		select {
		case <-listenerClosed:
			return
		case <-ctx.Done():
			_ = transport.Close()
		}
		drainRemoteForwardListener(listener, listenerClosed)
	}()
	return closing
}

func (closing *remoteListenerClose) wait(ctx context.Context) error {
	select {
	case <-closing.completed:
		return closing.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func drainRemoteForwardListener(listener net.Listener, listenerClosed <-chan struct{}) {
	for {
		select {
		case <-listenerClosed:
			return
		default:
		}
		connection, err := listener.Accept()
		if connection != nil {
			_ = connection.Close()
		}
		if err != nil {
			// A failed channel acknowledgement and a removed listener can both
			// return EOF. Keep draining until Close confirms removal; yielding
			// lets its goroutine finish when the closed queue returns immediately.
			runtime.Gosched()
		}
	}
}
