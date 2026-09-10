package sshclient

import (
	"context"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"sshc/internal/remoteos"
)

// Detect on a separate channel of the authenticated final host, never in the
// interactive shell. Failure must neither block readiness nor close transport.
func detectRemoteOS(ctx context.Context, client *ssh.Client) string {
	if strings.Contains(string(client.ServerVersion()), "OpenSSH_for_Windows") {
		return "windows"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	opened := make(chan *ssh.Session)
	go func() {
		session, err := client.NewSession()
		if err != nil {
			session = nil
		}
		select {
		case opened <- session:
		case <-ctx.Done():
			if session != nil {
				_ = session.Close()
			}
		}
	}()
	var session *ssh.Session
	select {
	case session = <-opened:
	case <-ctx.Done():
		return ""
	}
	if session == nil {
		return ""
	}
	defer session.Close()
	output := &osOutput{}
	session.Stdout, session.Stderr = output, io.Discard
	done := make(chan string, 1)
	go func() {
		// No interpolation, PTY, shell startup file changes, or sourcing os-release.
		_ = session.Run("uname -s; cat /etc/os-release 2>/dev/null")
		done <- remoteos.Parse(output.text.String())
	}()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return ""
	}
}

type osOutput struct{ text strings.Builder }

func (b *osOutput) Write(p []byte) (int, error) {
	if room := 8192 - b.text.Len(); room > 0 {
		_, _ = b.text.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
