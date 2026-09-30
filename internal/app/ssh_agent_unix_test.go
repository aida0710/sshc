//go:build unix

package app

import (
	"path/filepath"
	"testing"
)

// 接続の公開鍵認証と agent 転送は、Keys 画面と同じ keys.NewAgent で agent の宛先を
// 決める。Unix の宛先は SSH_AUTH_SOCK の指すソケットである。
func TestConnectionsReachTheAgentNamedBySSHAuthSock(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "agent.sock")
	t.Setenv("SSH_AUTH_SOCK", socket)

	parts := newSSHParts(sshDependencies{})

	if parts.dialer.Auth.Agent == nil {
		t.Fatal("connections were given no agent")
	}
	if got := parts.dialer.Auth.Agent.Address(); got != socket {
		t.Fatalf("connection agent = %q, want SSH_AUTH_SOCK %q", got, socket)
	}
}
