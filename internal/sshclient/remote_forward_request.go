package sshclient

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

type remoteForwardTransport interface {
	Listen(network, address string) (net.Listener, error)
	Close() error
}

func listenRemoteForward(client *ssh.Client, address string) (net.Listener, error) {
	// SSH global requests have no request IDs and cannot be individually
	// cancelled. Bound the server reply wait like connection establishment;
	// an unanswered request requires closing this transport before continuing.
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	return requestRemoteForward(ctx, client, address)
}

func requestRemoteForward(ctx context.Context, transport remoteForwardTransport, address string) (net.Listener, error) {
	listener, err := awaitRemoteForwardRequest(ctx, transport, func() (net.Listener, error) {
		return transport.Listen("tcp", address)
	})
	if ctx.Err() != nil && listener != nil {
		_ = transport.Close()
		_ = closeRemoteForwardListener(ctx, transport, listener)
		return nil, ctx.Err()
	}
	return listener, err
}

func closeRemoteForwardListener(ctx context.Context, transport remoteForwardTransport, listener net.Listener) error {
	return startRemoteListenerClose(ctx, transport, listener).wait(ctx)
}

func awaitRemoteForwardRequest(ctx context.Context, transport remoteForwardTransport, request func() (net.Listener, error)) (net.Listener, error) {
	type requestReply struct {
		listener net.Listener
		err      error
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The rendezvous transfers listener ownership. A buffered reply could
	// leave a successful late listener unread after the caller cancels.
	completed := make(chan requestReply)
	go func() {
		listener, err := request()
		select {
		case completed <- requestReply{listener: listener, err: err}:
		case <-ctx.Done():
			if listener != nil {
				_ = closeRemoteForwardListener(ctx, transport, listener)
			}
		}
	}()
	select {
	case reply := <-completed:
		return reply.listener, reply.err
	case <-ctx.Done():
		_ = transport.Close()
		return nil, ctx.Err()
	}
}
