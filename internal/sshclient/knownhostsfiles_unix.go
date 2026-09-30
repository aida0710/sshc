//go:build !windows

package sshclient

// DefaultGlobalKnownHostsFiles は、GlobalKnownHostsFile が書かれていないときの
// ファイルである。OpenSSH の既定（`ssh -G` の globalknownhostsfile）と同じ。
func DefaultGlobalKnownHostsFiles() []string {
	return []string{"/etc/ssh/ssh_known_hosts", "/etc/ssh/ssh_known_hosts2"}
}
