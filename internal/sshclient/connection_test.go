package sshclient_test

import (
	"context"
	"errors"
	"testing"

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
