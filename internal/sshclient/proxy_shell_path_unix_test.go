//go:build !windows && !android && !ios

package sshclient_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/platform"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

func TestSSHConnectsWithShellPathEvenWhenStartupPrintsABanner(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = channel.Write([]byte("connected through shell PATH\r\n"))
		},
	})
	home := t.TempDir()
	commands := filepath.Join(home, "commands with spaces")
	if err := os.Mkdir(commands, 0o700); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, "login-shell")
	for name, content := range map[string]string{
		shell: "#!/bin/sh\nprintf 'startup banner\\n'\nprintf 'startup stderr\\n' >&2\n" +
			"export PATH=\"$HOME/commands with spaces:$PATH\"\nexec /bin/sh -c \"$3\"\n",
		filepath.Join(commands, "proxy-relay"): "#!/bin/sh\nexec " + relayCommand(t, server.Address()) + "\n",
	} {
		if err := os.WriteFile(name, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	dialer := dialerFor(t, server, sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }})
	dialer.ProxyEnvironment = func(ctx context.Context) ([]string, error) {
		return platform.WithLoginShellPath(ctx, []string{"HOME=" + home, "SHELL=" + shell, "PATH=/usr/bin:/bin"})
	}
	target := targetWith(server, path)
	target.ProxyCommand = "proxy-relay"
	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "connected through shell PATH")
}
