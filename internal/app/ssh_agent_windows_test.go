//go:build windows

package app

import (
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowspipe"
)

// 接続の公開鍵認証と agent 転送は、Keys 画面と同じ keys.NewAgent で agent の宛先を
// 決める。Windows の宛先は、SSH_AUTH_SOCK があっても OpenSSH の固定の named pipe である。
func TestConnectionsReachTheOpenSSHAgentPipeEvenWithSSHAuthSockSet(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "agent.sock"))

	parts := newSSHParts(sshDependencies{})

	if parts.dialer.Auth.Agent == nil {
		t.Fatal("connections were given no agent")
	}
	if got := parts.dialer.Auth.Agent.Address(); got != windowspipe.AgentPipe {
		t.Fatalf("connection agent = %q, want the OpenSSH agent pipe %q", got, windowspipe.AgentPipe)
	}
}
