package sshclient_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestSubsystemConnectionRefusesAnUnknownHostWithoutPersistingIt(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	written := 0
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func(string) ([]byte, error) { return nil, nil },
			Add:  func(string, knownhosts.Candidate) error { written++; return nil },
		},
	}
	target := targetWith(server, path)
	target.Strict = "no"

	connection, err := dialer.Connect(context.Background(), target)
	if connection != nil {
		_ = connection.Close()
		t.Fatal("an unknown host returned an authenticated subsystem transport")
	}
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("Connect = %v, want ErrHostKeyUnknown", err)
	}
	if written != 0 {
		t.Fatal("a non-interactive subsystem persisted an unknown host key")
	}
}

func TestSubsystemConnectionUsesAStoredPasswordWithoutPrompting(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) {
		return "hunter2", true
	}}

	connection, err := dialerFor(t, server, auth).Connect(
		context.Background(), targetWith(server),
	)

	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
}

func TestSubsystemConnectionRefusesAnUnknownProxyJumpWithoutPersistingIt(t *testing.T) {
	path, contents, public := keyPair(t)
	inner := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	edge := newTestServer(t, serverOptions{
		AcceptKeys:       []ssh.PublicKey{public},
		AllowDirectTCPIP: true,
	})
	edge.allow(inner.Address())

	knownInner := knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())
	written := 0
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func(string) ([]byte, error) { return []byte(knownInner), nil },
			Add:  func(string, knownhosts.Candidate) error { written++; return nil },
		},
	}
	final := targetWith(inner, path)
	edgeTarget := targetWith(edge, path)
	edgeTarget.Strict = "no"
	final.Jump = []sshclient.Target{edgeTarget}

	connection, err := dialer.Connect(context.Background(), final)
	if connection != nil {
		_ = connection.Close()
		t.Fatal("an unknown ProxyJump returned an authenticated subsystem transport")
	}
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("Connect = %v, want ErrHostKeyUnknown", err)
	}
	if written != 0 {
		t.Fatal("a non-interactive subsystem persisted an unknown ProxyJump host key")
	}
}

func TestSubsystemConnectionSendsConfiguredKeepAlivesUntilClose(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.KeepAlive = 25 * time.Millisecond
	target.KeepAliveMax = 100

	connection, err := dialerFor(t, server, auth).Connect(context.Background(), target)
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	if !testwait.Reached(func() bool { return server.KeepAlives() >= 3 }) {
		_ = connection.Close()
		t.Fatalf("the server saw %d keepalives", server.KeepAlives())
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	// Close の直前に送った keepalive は、Close が戻ったあとでサーバーに数えられる
	// ことがある。数が増えないことではなく、サーバーから見て接続が終わることを
	// 確かめる。終わった接続から keepalive が届くことはない。
	if !testwait.Reached(func() bool { return server.Disconnected() >= 1 }) {
		t.Fatal("the connection stayed open after Close, so keepalives would continue")
	}
}

func TestSubsystemConnectionClosesWhenKeepAlivesGoUnanswered(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}, IgnoreKeepAlives: true})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.KeepAlive = 25 * time.Millisecond
	target.KeepAliveMax = 3

	connection, err := dialerFor(t, server, auth).Connect(context.Background(), target)
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	defer func() { _ = connection.Close() }()
	// Three unanswered probes at 25 ms each must close the transport well
	// before the TCP retransmission timeout would notice a dead peer.
	closed := make(chan struct{})
	go func() {
		_ = connection.Client().Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("the connection stayed open although the server never answered a keepalive")
	}
	// x/crypto/ssh serialises global requests, so the unanswered first probe
	// keeps the later ones from reaching the wire. Only the timeouts count.
	if got := server.KeepAlives(); got < 1 {
		t.Fatalf("the server saw %d keepalives before the close", got)
	}
}

// lockedLog は、接続ログを受け取る書き先である。keepalive も同じ書き先へ書くことがある。
type lockedLog struct {
	mutex sync.Mutex
	text  strings.Builder
}

func (log *lockedLog) Write(contents []byte) (int, error) {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.text.Write(contents)
}

func (log *lockedLog) String() string {
	log.mutex.Lock()
	defer log.mutex.Unlock()
	return log.text.String()
}

func TestSubsystemConnectionReportsItsStagesAndLogToAnObserver(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	dialer := dialerFor(t, server, sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }})
	dialer.Verbosity = func() connectionlog.Level { return connectionlog.Detailed }
	var phases []string
	log := &lockedLog{}
	ctx := sshclient.WithConnectionObserver(context.Background(), sshclient.ConnectionObserver{
		Progress: func(progress terminal.ConnectionProgress) { phases = append(phases, progress.Phase) },
		Log:      log,
	})

	connection, err := dialer.Connect(ctx, targetWith(server))
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	_ = connection.Close()

	want := []string{terminal.ConnectionDialing, terminal.ConnectionHostKey, terminal.ConnectionAuthenticating, terminal.ConnectionAuthenticated}
	if strings.Join(phases, ",") != strings.Join(want, ",") {
		t.Errorf("phases = %v, want %v", phases, want)
	}
	if !strings.Contains(log.String(), "[sshc][debug") || !strings.HasSuffix(log.String(), "\r\n") {
		t.Errorf("log = %q, want debug lines ending in CRLF as on a terminal", log.String())
	}
}

func TestSubsystemConnectionWithoutAnObserverWritesNothing(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	dialer := dialerFor(t, server, sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }})
	dialer.Verbosity = func() connectionlog.Level {
		t.Error("an unobserved connection read the log setting")
		return connectionlog.Full
	}
	connection, err := dialer.Connect(context.Background(), targetWith(server))
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	_ = connection.Close()
}
