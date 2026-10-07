//go:build !darwin

package sftp

import "syscall"

// isPrivacyProtectionRefusal is false outside macOS. Only macOS answers a
// protected folder with an errno that file permissions do not use, so
// elsewhere such a refusal cannot be told from a permission one.
func isPrivacyProtectionRefusal(syscall.Errno) bool {
	return false
}
