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
		_ = listener.Close()
		return nil, ctx.Err()
	}
	return listener, err
}

func closeRemoteForwardListener(ctx context.Context, transport remoteForwardTransport, listener net.Listener) error {
	_, err := awaitRemoteForwardRequest(ctx, transport, func() (struct{}, error) {
		return struct{}{}, listener.Close()
	})
	if ctx.Err() != nil {
		// Even an already cancelled stop must remove the local listener from
		// the SSH client after closing the transport.
		_ = transport.Close()
		_ = listener.Close()
	}
	return err
}

func awaitRemoteForwardRequest[T any](ctx context.Context, transport remoteForwardTransport, request func() (T, error)) (T, error) {
	type requestReply struct {
		value T
		err   error
	}
	if err := ctx.Err(); err != nil {
		var empty T
		return empty, err
	}
	completed := make(chan requestReply, 1)
	go func() {
		value, err := request()
		completed <- requestReply{value: value, err: err}
	}()
	select {
	case reply := <-completed:
		return reply.value, reply.err
	case <-ctx.Done():
		_ = transport.Close()
		reply := <-completed
		return reply.value, ctx.Err()
	}
}
