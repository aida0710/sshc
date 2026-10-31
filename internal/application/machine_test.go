package application

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
)

var testMachineStart = MachineStart{Hostname: "desk", Facts: MachineFacts{OS: "linux", Version: "v0.59.0", StartedAt: "2026-10-30T03:00:00Z"}}

// newPreparedService は、エンジンの起動と同じく PrepareMachine を済ませたサービスを返す。
func newPreparedService(t *testing.T) *Service {
	t.Helper()
	service, _ := newTerminalService(t)
	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestPreparingAMachineKeepsItsIDAcrossStarts(t *testing.T) {
	service, workspace := newTerminalService(t)
	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}
	first := service.MachineID()

	restarted := NewService(workspace, service.manager)
	if err := restarted.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	if first == "" || restarted.MachineID() != first {
		t.Fatalf("ids = %q then %q, want the same id", first, restarted.MachineID())
	}
}

func TestANewMachineIsNamedAfterItsHostAndListedFirst(t *testing.T) {
	service := newPreparedService(t)

	machines, err := service.Machines()
	if err != nil {
		t.Fatal(err)
	}

	if len(machines) != 1 || machines[0].ID != service.MachineID() || machines[0].Settings.Name != "desk" ||
		machines[0].Facts != testMachineStart.Facts {
		t.Fatalf("machines = %#v", machines)
	}
}

func TestThePortMovesFromTheOldEngineSettingsFile(t *testing.T) {
	service, workspace := newTerminalService(t)
	legacy := filepath.Join(workspace.StateDir(), legacyEngineSettingsFileName)
	if err := os.MkdirAll(workspace.StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, legacy, []byte(`{"schemaVersion":1,"port":50123,"vaultAutoLock":{"mode":"restart"}}`))

	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	if port := service.ThisMachineSettings().Port; port != 50123 {
		t.Fatalf("port = %d, want 50123", port)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("engine-settings.json still exists: %v", err)
	}
}

func TestPreparingAMachineMovesASchema9MetadataFile(t *testing.T) {
	service, workspace := newTerminalService(t)
	if err := service.metadata.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(metadataSchema9))

	if err := service.PrepareMachine(rand.Reader, testMachineStart); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(filepath.Join(workspace.StateDir(), MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `"schemaVersion": 10`) || strings.Contains(string(contents), "embeddedTerminal") {
		t.Fatalf("metadata.json was not rewritten in schema 10:\n%s", contents)
	}
}

func TestAMachineUsesThePresetItChose(t *testing.T) {
	service := newPreparedService(t)
	stored, _, _ := service.metadata.Load()
	phone := SettingsPreset{ID: "phone", Name: "スマホ", Terminal: &EmbeddedTerminal{MaxSessions: 3}}
	if _, err := service.SetPresets(stored.Presets, append(stored.Presets, phone)); err != nil {
		t.Fatal(err)
	}
	base := service.ThisMachineSettings()
	next := base
	next.PresetID = "phone"

	if _, err := service.SetMachineSettings(service.MachineID(), base, next); err != nil {
		t.Fatal(err)
	}

	if limits := service.TerminalLimits(); limits.MaxSessions != 3 {
		t.Fatalf("limits = %#v, want the phone preset's", limits)
	}
}

func TestAMachineWhosePresetWasRemovedUsesTheStandardOne(t *testing.T) {
	service := newPreparedService(t)
	stored, _, _ := service.metadata.Load()
	withPhone := append(stored.Presets, SettingsPreset{ID: "phone", Name: "スマホ", Terminal: &EmbeddedTerminal{MaxSessions: 3}})
	if _, err := service.SetPresets(stored.Presets, withPhone); err != nil {
		t.Fatal(err)
	}
	base := service.ThisMachineSettings()
	next := base
	next.PresetID = "phone"
	if _, err := service.SetMachineSettings(service.MachineID(), base, next); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetPresets(withPhone, withPhone[:1]); err != nil {
		t.Fatal(err)
	}

	document, err := service.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if document.PresetInUse != DefaultPresetID || service.EffectiveSettings().Preset.ID != DefaultPresetID {
		t.Fatalf("preset in use = %q, want the standard preset", document.PresetInUse)
	}
}

func TestAPortOutOfRangeIsRefused(t *testing.T) {
	service := newPreparedService(t)
	base := service.ThisMachineSettings()
	next := base
	next.Port = 80

	if _, err := service.SetMachineSettings(service.MachineID(), base, next); !errors.Is(err, ErrEnginePort) {
		t.Fatalf("err = %v, want ErrEnginePort", err)
	}
}

func TestThisMachineCannotBeRemovedFromTheList(t *testing.T) {
	service := newPreparedService(t)

	if err := service.RemoveMachine(service.MachineID()); !errors.Is(err, ErrThisMachine) {
		t.Fatalf("err = %v, want ErrThisMachine", err)
	}
}

func TestAnotherMachineIsRemovedWithItsState(t *testing.T) {
	service := newPreparedService(t)
	other, err := encodeMachineSettings(MachineSettings{Name: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := strings.Repeat("a", 32)
	acltest.WritePrivateFile(t, service.machineSettingsPath(otherID), other)
	state, err := encodeMachineState(MachineState{Facts: MachineFacts{OS: "darwin"}})
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, service.machineStatePath(otherID), state)

	if err := service.RemoveMachine(otherID); err != nil {
		t.Fatal(err)
	}

	machines, err := service.Machines()
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 {
		t.Fatalf("machines = %#v, want only this machine", machines)
	}
	if _, err := os.Stat(service.machineStatePath(otherID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the other machine's state was kept: %v", err)
	}
}

// 画面の並びはメモリに持ち、まとめて書く。書く前でも読めば変更が見え、FlushMachineState で書く。
func TestInterfaceStateIsHeldUntilItIsFlushed(t *testing.T) {
	service := newPreparedService(t)

	if err := service.SetInterfaceState(map[string]string{"runTarget": "server"}); err != nil {
		t.Fatal(err)
	}
	if got := service.ThisMachineState().Interface["runTarget"]; got != "server" {
		t.Fatalf("runTarget = %q before the flush, want server", got)
	}
	if written := service.machineState(service.MachineID()).Interface["runTarget"]; written != "" {
		t.Fatalf("runTarget was written at once: %q", written)
	}

	if err := service.FlushMachineState(); err != nil {
		t.Fatal(err)
	}
	if written := service.machineState(service.MachineID()).Interface["runTarget"]; written != "server" {
		t.Fatalf("runTarget written = %q, want server", written)
	}
}

func TestAnEmptyValueRemovesAnInterfaceState(t *testing.T) {
	service := newPreparedService(t)
	if err := service.SetInterfaceState(map[string]string{"sftpQueueView": "list"}); err != nil {
		t.Fatal(err)
	}

	if err := service.SetInterfaceState(map[string]string{"sftpQueueView": ""}); err != nil {
		t.Fatal(err)
	}

	if _, kept := service.ThisMachineState().Interface["sftpQueueView"]; kept {
		t.Fatal("an empty value did not remove the state")
	}
}

func TestAnUnknownInterfaceStateIsRefused(t *testing.T) {
	service := newPreparedService(t)

	if err := service.SetInterfaceState(map[string]string{"theme": "dark"}); !errors.Is(err, ErrMachineState) {
		t.Fatalf("err = %v, want ErrMachineState", err)
	}
}

// 前のバージョンのマシンが書いた今の状態（今は預からない sftpSplitRatio を持つ）も、残りの並びを保って読み、
// 同期で届いたファイルとしても通す。
func TestAStateWrittenByAnotherVersionIsReadWithoutTheNamesThisBuildDoesNotKeep(t *testing.T) {
	contents := []byte(`{"schemaVersion":1,"facts":{},"interface":{"runTarget":"server","sftpSplitRatio":"40"}}`)

	state, err := decodeMachineState(contents)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Interface) != 1 || state.Interface["runTarget"] != "server" {
		t.Fatalf("interface = %v, want only runTarget", state.Interface)
	}
	if err := validateMachineFile("0123456789abcdef0123456789abcdef.state.json", contents); err != nil {
		t.Fatalf("a synced state file was refused: %v", err)
	}
}

// 転送の処理を止めたことは、書く前にエンジンが落ちても止めたままにするため、すぐに書く。
func TestStoppingTransfersIsWrittenAtOnce(t *testing.T) {
	service := newPreparedService(t)

	if err := service.SetTransfersStopped(true); err != nil {
		t.Fatal(err)
	}

	if !service.machineState(service.MachineID()).TransfersStopped {
		t.Fatal("the stop was not written at once")
	}
}

// 転送キューで変えた設定は、プリセット・全体の設定・今の状態に分けて書く。
func TestTransferSettingsAreSplitBetweenThePresetTheGlobalSettingsAndTheState(t *testing.T) {
	service := newPreparedService(t)

	if err := service.SetTransferSettings(TransferSettings{
		Preset:  FileTransferSettings{MaxConcurrent: 4},
		Global:  GlobalFileTransferSettings{AutoReconnect: true},
		Stopped: true,
	}); err != nil {
		t.Fatal(err)
	}

	stored, _, _ := service.metadata.Load()
	if transfers := stored.Presets[0].FileTransfers; transfers == nil || transfers.MaxConcurrent != 4 {
		t.Fatalf("preset transfers = %#v", transfers)
	}
	if global := stored.Settings; global == nil || global.FileTransfers == nil || !global.FileTransfers.AutoReconnect {
		t.Fatalf("global transfers = %#v", stored.Settings)
	}
	if !service.TransferSettings().Stopped {
		t.Fatal("the stop was not kept in the state")
	}
}
