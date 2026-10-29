package sftp

import "golang.org/x/sys/unix"

// OpenBSD names the statfs fields after its C struct (f_bavail, f_bsize).
func statfsFreeBytes(stat *unix.Statfs_t) uint64 {
	return availableBlocks(stat.F_bavail) * uint64(stat.F_bsize)
}
