package sftp

import (
	"context"
	"errors"
	"testing"
)

type destinationRemote struct {
	*stubRemote
	destination string
}

func TestPoolDoesNotReuseAConnectionWhenTheAliasCannotBeResolved(t *testing.T) {
	var resolutionError error
	pool := NewRemotePool(func(context.Context, string) (RemoteTarget, error) {
		return RemoteTarget{Identity: "original", Open: func(context.Context) (Remote, error) {
			return &stubRemote{alive: true}, nil
		}}, resolutionError
	})
	defer pool.Close()
	first, err := pool.Open(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	resolutionError = errors.New("alias removed from configuration")
	if _, err := pool.Open(t.Context(), "edge"); !errors.Is(err, resolutionError) {
		t.Fatalf("cached connection bypasses resolution failure: %v", err)
	}
}

func (remote *destinationRemote) Getwd(context.Context) (string, error) {
	return remote.destination, nil
}

func TestPoolResolvesAnAliasAfterItsDestinationChanges(t *testing.T) {
	destination := "/old-server"
	pool := NewRemotePool(func(context.Context, string) (RemoteTarget, error) {
		resolved := destination
		return RemoteTarget{Identity: resolved, Open: func(context.Context) (Remote, error) {
			return &destinationRemote{stubRemote: &stubRemote{alive: true}, destination: resolved}, nil
		}}, nil
	})
	defer pool.Close()
	first, err := pool.Open(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	// The production opener resolves HostName/User/Port from current config.
	destination = "/new-server"
	next, err := pool.Open(t.Context(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	actual, err := next.Getwd(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if actual != destination {
		t.Fatalf("operation intended for %s still uses %s", destination, actual)
	}
}
