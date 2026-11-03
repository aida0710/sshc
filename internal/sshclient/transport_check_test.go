package sshclient_test

import (
	"context"
	"io"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// openShellForCheck は、シェルまで開いたセッションを返す。
func openShellForCheck(t *testing.T, options serverOptions) terminal.Process {
	t.Helper()
	path, contents, public := keyPair(t)
	options.AcceptKeys = []ssh.PublicKey{public}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	options.OnShell = func(channel ssh.Channel) {
		_, _ = io.WriteString(channel, "ready\r\n")
		<-release
	}
	server := newTestServer(t, options)
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	process, err := dialerFor(t, server, auth).Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "ready")
	drain(process)
	return process
}

func TestATransportThatDoesNotAnswerIsClosedAndEndsAsLost(t *testing.T) {
	process := openShellForCheck(t, serverOptions{IgnoreKeepAlives: true})

	process.(terminal.TransportChecking).CheckTransport(50 * time.Millisecond)

	done := make(chan terminal.ExitInfo, 1)
	go func() { done <- process.Wait() }()
	select {
	case info := <-done:
		if !info.TransportLost {
			t.Errorf("exit = %+v, want a lost transport", info)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the session kept waiting on a transport that does not answer")
	}
}

func TestATransportThatAnswersIsLeftAlone(t *testing.T) {
	process := openShellForCheck(t, serverOptions{})

	process.(terminal.TransportChecking).CheckTransport(5 * time.Second)

	done := make(chan terminal.ExitInfo, 1)
	go func() { done <- process.Wait() }()
	select {
	case info := <-done:
		t.Fatalf("the session ended: %+v", info)
	case <-time.After(100 * time.Millisecond):
	}
}
