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
