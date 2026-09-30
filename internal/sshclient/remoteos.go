package sshclient

import (
	"context"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/iowrite"
	"sshc/internal/remoteos"
)

const (
	// remoteOSDetectTimeout は、リモートOSの判定に待つ上限。判定はシェルの開始のあとに
	// 別のチャンネルで行う付け足しなので、応答の遅いホストでは長く待たずにあきらめる。
	remoteOSDetectTimeout = 3 * time.Second
	// remoteOSOutputLimit は、判定のために覚える出力の上限。uname と /etc/os-release は
	// ふつう 1 KiB に満たないので、これを超える分は判定に使わない。
	remoteOSOutputLimit = 8192
)

// Detect on a separate channel of the authenticated final host, never in the
// interactive shell. Failure must neither block readiness nor close transport.
func detectRemoteOS(ctx context.Context, client *ssh.Client) string {
	if strings.Contains(string(client.ServerVersion()), "OpenSSH_for_Windows") {
		return "windows"
	}
	ctx, cancel := context.WithTimeout(ctx, remoteOSDetectTimeout)
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
	output := iowrite.NewCappedBuffer(remoteOSOutputLimit)
	session.Stdout, session.Stderr = output, io.Discard
	done := make(chan string, 1)
	go func() {
		// No interpolation, PTY, shell startup file changes, or sourcing os-release.
		_ = session.Run("uname -s; cat /etc/os-release 2>/dev/null")
		done <- remoteos.Parse(output.String())
	}()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return ""
	}
}
