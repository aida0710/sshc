package application

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// newVaultClock は、service に Vault を取り付け、自動ロックの時計を読めるようにする。
func newVaultClock(t *testing.T, service *Service, workspace *storage.Workspace) *secret.Service {
	t.Helper()
	vault := secret.NewService(workspace,
		storage.NewManager(workspace, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x33}, 4096))),
		time.Now)
	service.SetVault(vault)
	return vault
}

func TestEngineSettingsAreNotWrittenToTheSyncedMetadata(t *testing.T) {
	service, workspace := newTerminalService(t)

	if _, err := service.SetEngineSettings(EngineSettings{
		Port:          43123,
		VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockRestart},
	}); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(filepath.Join(workspace.StateDir(), MetadataFileName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "43123") || strings.Contains(string(contents), "restart") {
		t.Fatalf("metadata.json carries this machine's engine settings to other machines:\n%s", contents)
	}
	if got := service.EngineSettings(); got.Port != 43123 || got.VaultAutoLock == nil ||
		got.VaultAutoLock.Mode != VaultAutoLockRestart {
		t.Fatalf("EngineSettings = %#v, want the saved port and auto-lock", got)
	}
}

func TestSavingEngineSettingsAppliesTheVaultClockImmediately(t *testing.T) {
	service, workspace := newTerminalService(t)
	vault := newVaultClock(t, service, workspace)

	if _, err := service.SetEngineSettings(EngineSettings{
		VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 30, Unit: VaultAutoLockMinutes},
	}); err != nil {
		t.Fatal(err)
	}
	if got := vault.IdleTimeout(); got != 30*time.Minute {
		t.Fatalf("IdleTimeout = %v, want 30m", got)
	}
	if _, err := service.SetEngineSettings(EngineSettings{
		VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockRestart},
	}); err != nil {
		t.Fatal(err)
	}
	if got := vault.IdleTimeout(); got != 0 {
		t.Fatalf("IdleTimeout = %v, want no automatic lock", got)
	}
}

func TestRestoringEngineSettingsFromTheHistoryAppliesTheVaultClock(t *testing.T) {
	service, workspace := newTerminalService(t)
	vault := newVaultClock(t, service, workspace)
	if _, err := service.SetEngineSettings(EngineSettings{
		VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 30, Unit: VaultAutoLockMinutes},
	}); err != nil {
		t.Fatal(err)
	}
	disabled, err := service.SetEngineSettings(EngineSettings{
		VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockRestart},
	})
	if err != nil {
		t.Fatal(err)
	}

	history, err := service.History()
	if err != nil {
		t.Fatal(err)
	}
	var restorable []string
	for _, entry := range history {
		if entry.ID == disabled.TransactionID {
			restorable = entry.Restorable
		}
	}
	if len(restorable) != 1 {
		t.Fatalf("restorable = %#v, want the engine settings before the change", restorable)
	}
	if _, err := service.Restore(disabled.TransactionID, restorable[0]); err != nil {
		t.Fatal(err)
	}

	if got := vault.IdleTimeout(); got != 30*time.Minute {
		t.Fatalf("IdleTimeout = %v, want the restored 30m", got)
	}
}

func TestVaultAutoLockRoundTripsAndConvertsItsUnit(t *testing.T) {
	for name, chosen := range map[string]struct {
		setting *VaultAutoLock
		want    time.Duration
	}{
		"default": {want: 12 * time.Hour},
		"minutes": {setting: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 45, Unit: VaultAutoLockMinutes}, want: 45 * time.Minute},
		"hours":   {setting: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 18, Unit: VaultAutoLockHours}, want: 18 * time.Hour},
		"restart": {setting: &VaultAutoLock{Mode: VaultAutoLockRestart}, want: 0},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := encodeEngineSettings(EngineSettings{VaultAutoLock: chosen.setting})
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeEngineSettings(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if got := decoded.VaultIdleTimeout(12 * time.Hour); got != chosen.want {
				t.Fatalf("VaultIdleTimeout = %v, want %v", got, chosen.want)
			}
		})
	}
}

func TestEngineSettingsOutsideTheirRangeAreRefusedWithTheirKind(t *testing.T) {
	service, _ := newTerminalService(t)

	for name, test := range map[string]struct {
		settings EngineSettings
		want     error
	}{
		"privileged port":         {EngineSettings{Port: 80}, ErrEnginePort},
		"just below the range":    {EngineSettings{Port: 1023}, ErrEnginePort},
		"port too large":          {EngineSettings{Port: 65536}, ErrEnginePort},
		"unknown mode":            {EngineSettings{VaultAutoLock: &VaultAutoLock{Mode: "forever"}}, ErrVaultAutoLock},
		"restart with a duration": {EngineSettings{VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockRestart, Value: 1, Unit: VaultAutoLockHours}}, ErrVaultAutoLock},
		"idle without a duration": {EngineSettings{VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockIdle}}, ErrVaultAutoLock},
		"idle too long":           {EngineSettings{VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 1000, Unit: VaultAutoLockHours}}, ErrVaultAutoLock},
		"unknown unit":            {EngineSettings{VaultAutoLock: &VaultAutoLock{Mode: VaultAutoLockIdle, Value: 1, Unit: "days"}}, ErrVaultAutoLock},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.SetEngineSettings(test.settings); !errors.Is(err, test.want) || !errors.Is(err, ErrEngineSettings) {
				t.Fatalf("error = %v, want %v within ErrEngineSettings", err, test.want)
			}
			if got := service.EngineSettings(); got != (EngineSettings{}) {
				t.Fatalf("the refusal wrote %#v", got)
			}
		})
	}
}
