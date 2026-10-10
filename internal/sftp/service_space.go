package sftp

import (
	"context"
	"math"
)

// FilesystemSpace reports bytes available to this account, rather than root's free blocks.
type FilesystemSpace struct {
	Path           string
	AvailableBytes uint64
	TotalBytes     uint64
}

func (s Service) FilesystemSpace(ctx context.Context, alias, remotePath string) (FilesystemSpace, error) {
	cleaned, err := cleanPublicPath(remotePath, true)
	if err != nil {
		return FilesystemSpace{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return FilesystemSpace{}, err
	}
	defer remote.Close()
	spaceRemote, supported := optionalRemote(remote).(SpaceRemote)
	if !supported {
		return FilesystemSpace{}, ErrUnsupportedOperation
	}
	stats, err := spaceRemote.StatVFS(cleaned)
	if err != nil {
		return FilesystemSpace{}, err
	}
	if stats == nil || stats.Frsize == 0 || stats.Blocks > math.MaxUint64/stats.Frsize || stats.Bavail > stats.Blocks {
		return FilesystemSpace{}, ErrInvalidSpace
	}
	return FilesystemSpace{Path: cleaned, AvailableBytes: stats.Bavail * stats.Frsize, TotalBytes: stats.Blocks * stats.Frsize}, nil
}
