package sshclient

import (
	"context"
	"errors"
	"net"
	"sync"

	"sshc/internal/terminal"
)

// ParseRemoteForward accepts a remote listen port followed by a destination
// reachable from the engine. Dynamic remote forwarding is not supported.
func ParseRemoteForward(value string) (ForwardSpec, error) {
	spec, err := ParseLocalForward(value)
	if err != nil {
		return ForwardSpec{}, err
	}
	host, port, err := net.SplitHostPort(spec.To)
	if err != nil || host == "" || !validPort(port) {
		return ForwardSpec{}, ErrInvalidForward
	}
	spec.Kind = terminal.ForwardRemote
	return spec, nil
}

// remoteForward owns the engine-side sockets as well as the SSH listener.
// Stopping only the listener would leave already accepted tunnels alive.
type remoteForward struct {
	listener    net.Listener
	transport   remoteForwardTransport
	destination string
	context     context.Context
	cancel      context.CancelFunc
	mutex       sync.Mutex
	connections map[net.Conn]struct{}
	workers     sync.WaitGroup
	closeOnce   sync.Once
	closeError  error
}

func newRemoteForward(listener net.Listener, destination string, transport remoteForwardTransport) *remoteForward {
	ctx, cancel := context.WithCancel(context.Background())
	forward := &remoteForward{
		listener:    listener,
		transport:   transport,
		destination: destination,
		context:     ctx,
		cancel:      cancel,
		connections: make(map[net.Conn]struct{}),
	}
	forward.workers.Add(1)
	return forward
}

func (forward *remoteForward) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	return forward.close(ctx)
}

func (forward *remoteForward) close(ctx context.Context) error {
	forward.closeOnce.Do(func() {
		// Closing a forwarded channel also writes to the SSH transport. Bound
		// the whole stop operation, including peers that no longer read packets.
		stopClosing := context.AfterFunc(ctx, func() { _ = forward.transport.Close() })
		defer stopClosing()
		forward.mutex.Lock()
		forward.cancel()
		connections := make([]net.Conn, 0, len(forward.connections))
		for connection := range forward.connections {
			connections = append(connections, connection)
		}
		forward.mutex.Unlock()
		for _, connection := range connections {
			_ = connection.Close()
		}
		forward.closeError = closeRemoteForwardListener(ctx, forward.transport, forward.listener)
		forward.workers.Wait()
		if err := ctx.Err(); err != nil {
			forward.closeError = err
		}
	})
	return forward.closeError
}

func (forward *remoteForward) accept() {
	defer forward.workers.Done()
	for {
		connection, err := forward.listener.Accept()
		if err != nil {
			return
		}
		// A server with GatewayPorts may disregard the loopback bind request.
		// Never pass an externally originated connection through to the engine.
		origin, _, err := net.SplitHostPort(connection.RemoteAddr().String())
		if err != nil || !isLoopback(origin) || !forward.register(connection) {
			_ = connection.Close()
			continue
		}
		go forward.serve(connection)
	}
}

func (forward *remoteForward) register(connection net.Conn) bool {
	forward.mutex.Lock()
	defer forward.mutex.Unlock()
	if forward.context.Err() != nil || len(forward.connections) >= maxConcurrentForwardConnections {
		return false
	}
	forward.connections[connection] = struct{}{}
	forward.workers.Add(1)
	return true
}

func (forward *remoteForward) serve(remote net.Conn) {
	defer forward.workers.Done()
	defer func() {
		_ = remote.Close()
		forward.mutex.Lock()
		delete(forward.connections, remote)
		forward.mutex.Unlock()
	}()
	// The same connection timeout used for SSH establishment bounds a local
	// destination that accepts no TCP connection. Cancellation ends it sooner.
	dialer := net.Dialer{Timeout: DefaultTimeout}
	local, err := dialer.DialContext(forward.context, "tcp", forward.destination)
	if err != nil {
		return
	}
	defer func() { _ = local.Close() }()
	stopClosing := context.AfterFunc(forward.context, func() { _ = local.Close() })
	defer stopClosing()
	relayForward(remote, local)
}

func remoteListenProblem(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return terminal.ForwardProblemRemoteTimeout
	}
	// SSH tcpip-forward returns a negative acknowledgement without a reason.
	// x/crypto/ssh exposes only this message, with no typed denial error; port
	// occupancy and server policy therefore cannot be distinguished.
	if err.Error() == "ssh: tcpip-forward request denied by peer" {
		return terminal.ForwardProblemRemoteDenied
	}
	return terminal.ForwardProblemFailed
}
