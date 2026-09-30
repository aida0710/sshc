package app

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"testing"
	"testing/fstest"
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

			if idle := services.vault.IdleTimeout(); idle != secret.IdleTimeout {
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
			// 設定の保存は、前の内容の控えを Vault の鍵で封じるので、Vault が要る。
			if err := first.vault.Initialise(""); err != nil {
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
			if got := services.vault.IdleTimeout(); got != test.want {
				t.Fatalf("IdleTimeout = %v, want %v", got, test.want)
			}
		})
	}
}

func TestEngineAutomaticallyOpensOnlyPasswordlessVaults(t *testing.T) {
	for _, password := range []string{"", "1234"} {
		t.Run(password, func(t *testing.T) {
			home := t.TempDir()
			first, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			if err := first.vault.Initialise(password); err != nil {
				t.Fatal(err)
			}
			first.vault.Lock()
			restarted, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.vault.Lock()
			if got := restarted.vault.Unlocked(); got != (password == "") {
				t.Fatalf("unlocked = %v", got)
			}
		})
	}
}

// engine は、受付を始めたことを伝えるとき、パスワードなしの Vault をロック解除済みとして
// 伝える。CLI はこれを見て、要らない sshc vault unlock を案内しない。
func TestReadinessReportsOnlyAPasswordlessVaultAsUnlocked(t *testing.T) {
	for _, password := range []string{"", "1234"} {
		t.Run(password, func(t *testing.T) {
			home := t.TempDir()
			first, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
			if err != nil {
				t.Fatal(err)
			}
			if err := first.vault.Initialise(password); err != nil {
				t.Fatal(err)
			}
			first.vault.Lock()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			announced := make(chan Readiness, 1)
			dependencies := Dependencies{
				Random: rand.Reader,
				Announce: func(readiness Readiness) error {
					announced <- readiness
					return nil
				},
				Listen: net.Listen,
				UI:     fstest.MapFS{"index.html": {Data: []byte("ok")}},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Home:   home,
				Owner:  handoff.OwnerEngine,
				PID:    4242,
			}
			done := make(chan error, 1)
			go func() { done <- Run(ctx, dependencies, "test") }()

			readiness := <-announced
			cancel()
			if err := <-done; err != nil {
				t.Fatalf("Run = %v", err)
			}
			if !readiness.VaultExists || readiness.VaultUnlocked != (password == "") {
				t.Fatalf("readiness vault exists=%v unlocked=%v", readiness.VaultExists, readiness.VaultUnlocked)
			}
		})
	}
}
