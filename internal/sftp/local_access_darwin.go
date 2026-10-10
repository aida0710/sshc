//go:build darwin

package sftp

import "syscall"

// isPrivacyProtectionRefusal reports whether errno is macOS's privacy
// protection refusing access. It answers EPERM ("Operation not permitted") for
// a folder the user has not allowed, while file permissions answer EACCES.
// System Integrity Protection and the immutable flag answer EPERM too, but
// they guard system files and rarely meet the pane in the user's folders.
func isPrivacyProtectionRefusal(errno syscall.Errno) bool {
	return errno == syscall.EPERM
}
