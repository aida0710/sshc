//go:build !windows && !openbsd

package sftp

import "golang.org/x/sys/unix"

func statfsFreeBytes(stat *unix.Statfs_t) uint64 {
	return availableBlocks(stat.Bavail) * uint64(stat.Bsize)
}
