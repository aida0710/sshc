package sftp

import pkgsftp "github.com/pkg/sftp"

// Optional operations stay outside Remote so read-only remotes need not implement them.
type SymlinkRemote interface {
	Symlink(target, linkPath string) error
}
type AtomicSymlinkRemote interface {
	ReplaceSymlink(temporary, linkPath string) error
}
type OwnershipRemote interface {
	Chown(path string, uid, gid uint32) error
}
type SpaceRemote interface {
	StatVFS(path string) (*pkgsftp.StatVFS, error)
}

// UnderlyingRemote lets connection-lifetime wrappers preserve optional operations.
type RemoteWrapper interface{ UnderlyingRemote() Remote }

func optionalRemote(remote Remote) Remote {
	for {
		wrapper, ok := remote.(RemoteWrapper)
		if !ok {
			return remote
		}
		remote = wrapper.UnderlyingRemote()
	}
}

func (remote *contextRemote) UnderlyingRemote() Remote { return remote.Remote }
func (remote *pooledRemote) UnderlyingRemote() Remote  { return remote.Remote }
