package app

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestTheEngineRestoresTheConfiguredVaultClock(t *testing.T) {
	for name, document := range map[string]struct {
		autoLock string
		want     time.Duration
	}{
		"minutes": {autoLock: `{"mode":"idle","value":45,"unit":"minutes"}`, want: 45 * time.Minute},
		"restart": {autoLock: `{"mode":"restart"}`, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			state := filepath.Join(home, ".ssh", "sshc")
			if err := os.MkdirAll(state, 0o700); err != nil {
				t.Fatal(err)
			}
			contents := []byte(`{"schemaVersion":4,"engine":{"vaultAutoLock":` + document.autoLock + `}}`)
			if err := os.WriteFile(filepath.Join(state, "metadata.json"), contents, 0o600); err != nil {
				t.Fatal(err)
			}
			services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			if got := services.passwords.IdleTimeout(); got != document.want {
				t.Fatalf("IdleTimeout = %v, want %v", got, document.want)
			}
		})
	}
}
