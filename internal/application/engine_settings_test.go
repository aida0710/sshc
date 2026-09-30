package application

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl/acltest"
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

// このバージョンで初めて起動したときに、前のバージョンが metadata.json に書いた
// engine 節をこのマシンの設定として移す。
func TestTheFirstStartMovesTheEngineSectionOfAnOlderMetadata(t *testing.T) {
	service, _ := newTerminalService(t)
	older := []byte(`{"schemaVersion":8,"engine":{"port":43123,` +
		`"vaultAutoLock":{"mode":"idle","value":45,"unit":"minutes"}}}`)
	acltest.WritePrivateFile(t, service.metadata.Path(), older)

	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	got := service.EngineSettings()
	if got.Port != 43123 || got.VaultAutoLock == nil || got.VaultAutoLock.Value != 45 ||
		got.VaultAutoLock.Unit != VaultAutoLockMinutes {
		t.Fatalf("EngineSettings = %#v, want the engine section of the older metadata.json", got)
	}
	contents, err := os.ReadFile(service.metadata.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, older) {
		t.Fatalf("metadata.json was rewritten:\n%s", contents)
	}
}

func TestAnEngineSectionThatArrivesAfterTheFirstStartIsNotMoved(t *testing.T) {
	service, _ := newTerminalService(t)
	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}
	// 同期や履歴の復元で、前の形の metadata.json が届く。
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(`{"schemaVersion":8,"engine":{"port":60000}}`))

	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	if got := service.EngineSettings(); got != (EngineSettings{}) {
		t.Fatalf("EngineSettings = %#v, want this machine's (none)", got)
	}
}

func TestAnEngineSectionInTheCurrentSchemaIsNotMoved(t *testing.T) {
	service, _ := newTerminalService(t)
	acltest.WritePrivateFile(t, service.metadata.Path(),
		[]byte(fmt.Sprintf(`{"schemaVersion":%d,"engine":{"port":60000}}`, MetadataSchemaVersion)))

	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	if got := service.EngineSettings(); got != (EngineSettings{}) {
		t.Fatalf("EngineSettings = %#v, want none", got)
	}
}

func TestOutOfRangeValuesInAnOlderEngineSectionFallBackToTheDefaults(t *testing.T) {
	service, _ := newTerminalService(t)
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(`{"schemaVersion":8,"engine":{"port":80,`+
		`"vaultAutoLock":{"mode":"idle","value":30,"unit":"minutes"}}}`))

	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	got := service.EngineSettings()
	if got.Port != 0 || got.VaultAutoLock == nil || got.VaultAutoLock.Value != 30 {
		t.Fatalf("EngineSettings = %#v, want the default port and the valid auto-lock", got)
	}
}

func TestAnUnreadableMetadataIsMovedOnALaterStart(t *testing.T) {
	service, _ := newTerminalService(t)
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(`{"schemaVersion":8,"engine":{"port":43123}`))
	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	// 利用者が metadata.json を直してから起動し直す。
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(`{"schemaVersion":8,"engine":{"port":43123}}`))
	if err := service.InitialiseEngineSettings(); err != nil {
		t.Fatal(err)
	}

	if got := service.EngineSettings().Port; got != 43123 {
		t.Fatalf("port = %d, want the port of the repaired metadata.json", got)
	}
}
