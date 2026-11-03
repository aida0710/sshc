package sshclient_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// tmuxServer は、tmux の有無と動いているセッションを決めておくサーバーである。
func tmuxServer(t *testing.T, installed, running bool) (*testServer, sshclient.Dialer, sshclient.Target) {
	t.Helper()
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "in-a-shell\r\n") },
		OnExec:     func(channel ssh.Channel) { _, _ = io.WriteString(channel, "in-tmux\r\n") },
		AnswerExec: func(command string) (string, uint32, bool) {
			switch {
			case strings.Contains(command, "tmux -V"):
				if !installed {
					return "", 127, true
				}
				return "tmux 3.5a\n", 0, true
			case strings.Contains(command, "has-session"):
				if running {
					return "", 0, true
				}
				return "", 1, true
			}
			return "", 0, false
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.TmuxSession = "sshc-web-01"
	return server, dialerFor(t, server, auth), target
}

func openForTest(t *testing.T, dialer sshclient.Dialer, target sshclient.Target) terminal.Process {
	t.Helper()
	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	return process
}

func TestAConnectionThatOpensInTmuxStartsANewTmuxSession(t *testing.T) {
	server, dialer, target := tmuxServer(t, true, false)
	process := openForTest(t, dialer, target)

	seen := readUntil(t, process, "in-tmux")
	if !strings.Contains(seen, "tmuxのセッションsshc-web-01を新しく開きます") {
		t.Errorf("the terminal does not say what it opened: %q", seen)
	}
	if command := server.Command(); !strings.Contains(command, "tmux new -A -s sshc-web-01") {
		t.Errorf("started %q", command)
	}
	if process.(terminal.TmuxResuming).ResumedTmuxSession() {
		t.Error("a new tmux session was reported as resumed")
	}
}

func TestAConnectionReturnsToTheTmuxSessionStillRunning(t *testing.T) {
	_, dialer, target := tmuxServer(t, true, true)
	process := openForTest(t, dialer, target)

	seen := readUntil(t, process, "in-tmux")
	if !strings.Contains(seen, "tmuxのセッションsshc-web-01に戻ります") {
		t.Errorf("the terminal does not say it returned: %q", seen)
	}
	if !process.(terminal.TmuxResuming).ResumedTmuxSession() {
		t.Error("the session it returned to was not reported as resumed")
	}
}

func TestAServerWithoutTmuxOpensAShellAndSaysSo(t *testing.T) {
	_, dialer, target := tmuxServer(t, false, false)
	process := openForTest(t, dialer, target)

	seen := readUntil(t, process, "in-a-shell")
	if !strings.Contains(seen, "tmuxが見つからない") {
		t.Errorf("the terminal does not say tmux is missing: %q", seen)
	}
}

// Host ブロックの RemoteCommand は利用者が決めた起動のしかたなので、それに従う。
func TestARemoteCommandInTheHostBlockIsNotReplacedByTmux(t *testing.T) {
	server, dialer, target := tmuxServer(t, true, false)
	target.RemoteCommand = "htop"
	process := openForTest(t, dialer, target)

	seen := readUntil(t, process, "in-tmux")
	if !strings.Contains(seen, "RemoteCommandがある") {
		t.Errorf("the terminal does not say why: %q", seen)
	}
	if command := server.Command(); command != "htop" {
		t.Errorf("started %q", command)
	}
}

func TestAConnectionWithoutTheSettingDoesNotLookForTmux(t *testing.T) {
	server, dialer, target := tmuxServer(t, true, true)
	target.TmuxSession = ""
	process := openForTest(t, dialer, target)

	readUntil(t, process, "in-a-shell")
	if commands := server.Commands(); len(commands) != 0 {
		t.Errorf("ran %q on the server", commands)
	}
}
