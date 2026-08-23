package app

import (
	"crypto/rand"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/secret"
)

func TestTheVaultClosesOnTheSameClockWhoeverStartedTheEngine(t *testing.T) {
	for _, owner := range []handoff.Owner{handoff.OwnerEngine, handoff.OwnerEngine} {
		t.Run(string(owner), func(t *testing.T) {
			services, err := newEngineServices(Dependencies{
				Home:   t.TempDir(),
				Owner:  owner,
				Random: rand.Reader,
			})
			if err != nil {
				t.Fatal(err)
			}

			if idle := services.passwords.IdleTimeout(); idle != secret.IdleTimeout {
				t.Errorf("%s の engine は %v で閉じるべきだが、idle=%v だった",
					owner, secret.IdleTimeout, idle)
			}
		})
	}
}

func TestTheClockIsTwelveHours(t *testing.T) {
	if hours := secret.IdleTimeout.Hours(); hours != 12 {
		t.Fatalf("IdleTimeout = %v 時間, want 12", hours)
	}
}
