package sftp

import "context"

// RemoteTarget describes one resolved configuration. Identity changes whenever
// its peer, route, or connection settings change; Open must use this snapshot.
type RemoteTarget struct {
	Identity string
	Open     func(context.Context) (Remote, error)
}

type ResolveRemote func(context.Context, string) (RemoteTarget, error)
