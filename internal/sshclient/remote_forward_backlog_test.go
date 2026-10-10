package sshclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// Product requests still use DefaultTimeout. Only this injected context expires quickly.
const (
	remoteForwardBacklogDeadline = 100 * time.Millisecond
	// One acknowledgement in flight plus more than the one-channel listener buffer.
	remoteForwardBacklogChannels = 5
	// Include every test goroutine when locating the pending channel send barrier.
	remoteForwardBacklogStackBytes = 256 << 10
)

// Pause encrypted writes while still reading incoming forwarded channels. This
// recreates a peer that sends connections but no longer reads SSH acknowledgements.
type pausedForwardWrites struct {
	net.Conn
	paused    atomic.Bool
	closing   chan struct{}
	started   chan struct{}
	closeOnce sync.Once
	startOnce sync.Once
}

func (connection *pausedForwardWrites) Write(contents []byte) (int, error) {
	if connection.paused.Load() {
		connection.startOnce.Do(func() { close(connection.started) })
		<-connection.closing
		return 0, net.ErrClosed
	}
	return connection.Conn.Write(contents)
}

func (connection *pausedForwardWrites) Close() error {
	connection.closeOnce.Do(func() { close(connection.closing) })
	return connection.Conn.Close()
}

type remoteForwardBacklog struct {
	client   *ssh.Client
	forward  *remoteForward
	incoming sync.WaitGroup
}

func pendingRemoteForwardBacklog(t *testing.T) *remoteForwardBacklog {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	serverReady := make(chan *ssh.ServerConn, 1)
	serverFailed := make(chan error, 1)
	go func() {
		incoming, err := listener.Accept()
		if err != nil {
			serverFailed <- err
			return
		}
		server, channels, requests, err := ssh.NewServerConn(incoming, serverConfig)
		if err != nil {
			serverFailed <- err
			return
		}
		serverReady <- server
		go func() {
			for channel := range channels {
				_ = channel.Reject(ssh.UnknownChannelType, "unused")
			}
		}()
		for request := range requests {
			_ = request.Reply(true, nil)
		}
	}()
	raw, err := net.DialTimeout("tcp", listener.Addr().String(), remoteForwardRequestTestDeadline)
	if err != nil {
		t.Fatal(err)
	}
	controlled := &pausedForwardWrites{Conn: raw, closing: make(chan struct{}), started: make(chan struct{})}
	t.Cleanup(func() { _ = controlled.Close() })
	// Both endpoints are created by this test on an isolated loopback listener.
	connection, channels, requests, err := ssh.NewClientConn(controlled, listener.Addr().String(), &ssh.ClientConfig{User: "isolated-review", HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if err != nil {
		t.Fatal(err)
	}
	client := ssh.NewClient(connection, channels, requests)
	t.Cleanup(func() { _ = client.Close() })
	var server *ssh.ServerConn
	select {
	case server = <-serverReady:
	case err := <-serverFailed:
		t.Fatal(err)
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("isolated SSH server did not connect")
	}
	t.Cleanup(func() { _ = server.Close() })
	remote, err := client.Listen("tcp", "127.0.0.1:9080")
	if err != nil {
		t.Fatal(err)
	}
	controlled.paused.Store(true)
	backlog := &remoteForwardBacklog{client: client}
	forward := newRemoteForward(remote, "127.0.0.1:1", client)
	backlog.forward = forward
	go forward.accept()
	payload := ssh.Marshal(struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}{"127.0.0.1", 9080, "127.0.0.1", 12345})
	// More than the SSH listener's one queued channel: acknowledgement stalls
	// while its producer holds the registry mutex needed by listener.Close.
	for range remoteForwardBacklogChannels {
		backlog.incoming.Add(1)
		go func() {
			defer backlog.incoming.Done()
			channel, _, err := server.OpenChannel("forwarded-tcpip", payload)
			if err == nil {
				_ = channel.Close()
			}
		}()
	}
	select {
	case <-controlled.started:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("forwarded channel acknowledgement did not stall")
	}
	waitForForwardQueueBacklog(t)
	return backlog
}

func waitForForwardQueueBacklog(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(remoteForwardRequestTestDeadline)
	for time.Now().Before(deadline) {
		stack := make([]byte, remoteForwardBacklogStackBytes)
		length := runtime.Stack(stack, true)
		for _, frame := range strings.Split(string(stack[:length]), "\n\n") {
			if strings.Contains(frame, "[chan send]") && strings.Contains(frame, "(*forwardList).forward") {
				return
			}
		}
		runtime.Gosched()
	}
	t.Fatal("SSH listener did not develop its pending-channel backlog")
}

func TestRemoteListenerStopDrainsQueuedChannelsAfterItsDeadline(t *testing.T) {
	backlog := pendingRemoteForwardBacklog(t)
	ctx, cancel := context.WithTimeout(context.Background(), remoteForwardBacklogDeadline)
	defer cancel()
	returned := make(chan error, 1)
	go func() { returned <- backlog.forward.close(ctx) }()
	select {
	case err := <-returned:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expired listener close = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("listener close exceeded its injected deadline")
	}
	assertRemoteBacklogClosed(t, backlog)
}

func TestSessionShutdownFinishesAPreviouslyStartedRemoteStop(t *testing.T) {
	backlog := pendingRemoteForwardBacklog(t)
	stopped := make(chan error, 1)
	go func() { stopped <- backlog.forward.close(context.Background()) }()
	select {
	case <-backlog.forward.context.Done():
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("first stop did not begin")
	}
	_ = backlog.client.Close()
	shutdown := make(chan error, 1)
	go func() { shutdown <- backlog.forward.closeAfterTransport() }()
	select {
	case err := <-shutdown:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown close = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("session shutdown waited on a concurrent stop")
	}
	select {
	case err := <-stopped:
		// The first stop may observe transport failure before the second
		// caller cancels its context. Either outcome must finish the cleanup.
		if !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
			t.Fatalf("first stop after shutdown = %v", err)
		}
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("session shutdown retained the first stop or its listener cleanup")
	}
	assertRemoteBacklogClosed(t, backlog)
}

func assertRemoteBacklogClosed(t *testing.T, backlog *remoteForwardBacklog) {
	t.Helper()
	select {
	case <-backlog.forward.closed:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("listener cleanup retained a blocked request or channel drain")
	}
	incomingDone := make(chan struct{})
	go func() { backlog.incoming.Wait(); close(incomingDone) }()
	select {
	case <-incomingDone:
	case <-time.After(remoteForwardRequestTestDeadline):
		t.Fatal("queued SSH channels survived listener cleanup")
	}
}
