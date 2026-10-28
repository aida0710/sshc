//go:build !windows && !linux

package nofollow

import (
	"golang.org/x/sys/unix"
)

// O_PATHがないUnixでは従来どおりreadable directory descriptorを使う。
func walkDirectoryFlags(bool) int {
	return unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_DIRECTORY
}
