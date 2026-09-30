//go:build windows

package sshclient

import (
	"os"
	"path/filepath"
)

// DefaultGlobalKnownHostsFiles は、GlobalKnownHostsFile が書かれていないときの
// ファイルである。Windows 版の OpenSSH は %ProgramData%\ssh に置く。
func DefaultGlobalKnownHostsFiles() []string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return nil
	}
	return []string{
		filepath.Join(programData, "ssh", "ssh_known_hosts"),
		filepath.Join(programData, "ssh", "ssh_known_hosts2"),
	}
}
