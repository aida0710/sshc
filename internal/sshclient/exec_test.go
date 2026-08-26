package sshclient_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
)

func TestRunSendsStdinAndReadsTheOutput(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			// 届いたものをそのまま返す。argv ではなく stdin を通ったことを、
			// これで見る。
			received, _ := io.ReadAll(channel)
			_, _ = channel.Write([]byte("received:" + string(received)))
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	output, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server, path), "install-key", []byte("ssh-ed25519 AAAA fixture\n"))
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if !strings.Contains(string(output.Stdout), "ssh-ed25519 AAAA fixture") {
		t.Fatalf("stdout = %q", output.Stdout)
	}
	// 公開鍵が argv に乗ることは決してない。
	if strings.Contains(server.Command(), "ssh-ed25519") {
		t.Fatalf("the key reached the command line: %q", server.Command())
	}
	if server.Command() != "install-key" {
		t.Errorf("command = %q", server.Command())
	}
	if server.ShellRan() {
		t.Error("Run started a shell instead of a command")
	}
}

// 非対話処理なので端末を要求しない。
func TestRunRequestsNoTerminal(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	if _, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server, path), "true", nil,
	); err != nil {
		t.Fatalf("Run = %v", err)
	}
	if term, _ := server.PTY(); term != "" {
		t.Fatalf("Run asked for a terminal: %q", term)
	}
}

// 終了コードは結果であって失敗ではない。リモートが応答したのだから、その結果を返す。
func TestRunReportsANonZeroExitAsAResult(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}, ExitCode: 3})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	output, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server, path), "false", nil)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if output.ExitCode != 3 {
		t.Fatalf("exit = %d", output.ExitCode)
	}
}

// 相手が延々と喋っても、取り込む量には上限がある。
func TestRunCapsWhatItTakesIn(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			noise := strings.Repeat("noisy line\n", 64<<10)
			_, _ = io.WriteString(channel, noise)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	output, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server, path), "talk", nil)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if len(output.Stdout) > sshclient.MaxCapturedOutput {
		t.Fatalf("stdout = %d bytes, past the ceiling", len(output.Stdout))
	}
	if !output.Truncated {
		t.Fatal("the output was cut but not reported as truncated")
	}
}

// 非対話処理なのでユーザー入力を待たない。
func TestRunNeverAsksTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	// パスワード認証しか受け付けないサーバーに、尋ねる手段の無い接続で挑む。
	if _, err := dialerFor(t, server, sshclient.Auth{}).Run(
		context.Background(), targetWith(server), "true", nil,
	); err == nil {
		t.Fatal("Run authenticated without anything to offer")
	}
	if attempts := server.Attempts(); attempts != 0 {
		t.Fatalf("Run offered a credential %d time(s)", attempts)
	}
}

func TestRunUsesAStoredPasswordWithoutAskingTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) {
		return "hunter2", true
	}}

	output, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server), "true", nil,
	)

	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if output.ExitCode != 0 {
		t.Fatalf("exit = %d, want 0", output.ExitCode)
	}
}

func TestRunStopsWhenTheContextIsDone(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(ssh.Channel) { <-release },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, err := dialerFor(t, server, auth).Run(ctx, targetWith(server, path), "sleep forever", nil)
		finished <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && server.Command() == "" {
		time.Sleep(10 * time.Millisecond)
	}
	if server.Command() == "" {
		t.Fatal("the remote command never started")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

func TestRunUsesFailureExitWhenTheRemoteOmitsAStatus(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public}, OmitExitStatus: true,
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	output, err := dialerFor(t, server, auth).Run(
		context.Background(), targetWith(server, path), "broken", nil,
	)
	if err == nil {
		t.Fatal("Run accepted a command result without an exit status")
	}
	if output.ExitCode != sshclient.RemoteFailureExit {
		t.Fatalf("exit = %d, want %d", output.ExitCode, sshclient.RemoteFailureExit)
	}
}

func TestRunRefusesAnUnknownProxyJumpWithoutPersistingIt(t *testing.T) {
	path, contents, public := keyPair(t)
	inner := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	edge := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public}, AllowDirectTCPIP: true,
	})
	edge.allow(inner.Address())
	written := 0
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) {
				return []byte(knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())), nil
			},
			Add: func(knownhosts.Candidate) error { written++; return nil },
		},
	}
	target := targetWith(inner, path)
	jump := targetWith(edge, path)
	jump.Strict = "no"
	target.Jump = []sshclient.Target{jump}

	output, err := dialer.Run(context.Background(), target, "true", nil)
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("Run = %v, want ErrHostKeyUnknown", err)
	}
	if output.ExitCode != sshclient.RemoteFailureExit {
		t.Fatalf("exit = %d, want %d", output.ExitCode, sshclient.RemoteFailureExit)
	}
	if written != 0 {
		t.Fatal("Run persisted an unknown ProxyJump host key")
	}
}
