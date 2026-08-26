package sshclient_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

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
