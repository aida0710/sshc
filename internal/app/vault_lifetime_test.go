package app

import (
	"crypto/rand"
	"testing"
	"time"

	"sshc/internal/application"
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
	for name, test := range map[string]struct {
		autoLock application.VaultAutoLock
		want     time.Duration
	}{
		"minutes": {
			autoLock: application.VaultAutoLock{
				Mode: application.VaultAutoLockIdle, Value: 45, Unit: application.VaultAutoLockMinutes,
			},
			want: 45 * time.Minute,
		},
		"restart": {
			autoLock: application.VaultAutoLock{Mode: application.VaultAutoLockRestart},
			want:     0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			first, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := first.config.SetEngineSettings(application.EngineSettings{
				VaultAutoLock: &test.autoLock,
			}); err != nil {
				t.Fatal(err)
			}
			services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			if got := services.passwords.IdleTimeout(); got != test.want {
				t.Fatalf("IdleTimeout = %v, want %v", got, test.want)
			}
		})
	}
}
