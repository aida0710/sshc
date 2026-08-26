package sshclient_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
)

func TestSubsystemConnectionRefusesAnUnknownHostWithoutPersistingIt(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	written := 0
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return nil, nil },
			Add:  func(knownhosts.Candidate) error { written++; return nil },
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
			Read: func() ([]byte, error) { return []byte(knownInner), nil },
			Add:  func(knownhosts.Candidate) error { written++; return nil },
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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && server.KeepAlives() < 3 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := server.KeepAlives(); got < 3 {
		_ = connection.Close()
		t.Fatalf("the server saw %d keepalives", got)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	afterClose := server.KeepAlives()
	time.Sleep(100 * time.Millisecond)
	if got := server.KeepAlives(); got != afterClose {
		t.Fatalf("keepalives continued after Close: %d -> %d", afterClose, got)
	}
}
