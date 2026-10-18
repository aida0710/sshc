package app

import (
	"errors"
	"fmt"
	"testing"

	"sshc/internal/vpn"
)

func TestOrphanDiscardWarnsOnlyWhenDockerRunsButCleanupFails(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nothing to discard", err: nil, want: false},
		{name: "docker is not installed", err: fmt.Errorf("%w: docker", vpn.ErrDockerMissing), want: false},
		{name: "docker desktop is stopped", err: fmt.Errorf("%w: docker.exe: open //./pipe/dockerDesktopLinuxEngine", vpn.ErrDockerNotRunning), want: false},
		{name: "docker refused to list containers", err: errors.New("permission denied while trying to connect to the docker API"), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := orphanDiscardNeedsAttention(test.err); got != test.want {
				t.Fatalf("orphanDiscardNeedsAttention(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
