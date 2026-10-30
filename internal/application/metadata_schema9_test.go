package application

import (
	"bytes"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
)

// schema 9 の metadata.json の例。設定は embeddedTerminal・fileTransfers・backgrounds・
// appearance・terminalPresets にあった。
const metadataSchema9 = `{
  "schemaVersion": 9,
  "groupsFile": "groups.sshc.conf",
  "embeddedTerminal": {
    "maxSessions": 8, "fontSize": 15, "verbosity": 2, "reconnect": 0, "osc52": true,
    "startDirectory": "~/work", "localShellProfile": "zsh", "ligatures": true
  },
  "fileTransfers": {
    "maxConcurrent": 3, "clearCompletedAfterSeconds": 60, "processingStopped": true,
    "autoReconnect": true, "maxReconnectAttempts": 4, "excludePatterns": ["node_modules"]
  },
  "backgrounds": {"capacityMiB": 64},
  "appearance": {"accent": "sky"},
  "terminalPresets": [{"id": "phone", "name": "スマホ", "settings": {"fontSize": 12, "composerEnterSends": true}}],
  "hosts": [{"identity": {"path": "config", "alias": "bastion"}, "tags": ["prod"]}]
}`

func TestSchema9SettingsMoveToTheGlobalSettingsAndTheStandardPreset(t *testing.T) {
	migrated, err := migrateMetadataFromSchema9([]byte(metadataSchema9))
	if err != nil {
		t.Fatal(err)
	}

	global := migrated.Settings
	if global == nil || global.Terminal == nil || global.Terminal.Verbosity != 2 || global.Terminal.Reconnect == nil ||
		*global.Terminal.Reconnect != 0 || !global.Terminal.OSC52 {
		t.Fatalf("global terminal = %#v", global)
	}
	if transfers := global.FileTransfers; transfers == nil || transfers.ClearCompletedAfterSeconds != 60 ||
		!transfers.AutoReconnect || transfers.MaxReconnectAttempts != 4 {
		t.Fatalf("global transfers = %#v", global.FileTransfers)
	}
	if global.Backgrounds == nil || global.Backgrounds.CapacityMiB != 64 {
		t.Fatalf("backgrounds = %#v", global.Backgrounds)
	}
	standard := migrated.Presets[0]
	if standard.ID != DefaultPresetID || standard.Appearance == nil || standard.Appearance.Accent != "sky" {
		t.Fatalf("standard preset = %#v", standard)
	}
	if terminal := standard.Terminal; terminal == nil || terminal.MaxSessions != 8 || terminal.FontSize != 15 ||
		terminal.StartDirectory != "~/work" || !terminal.Ligatures {
		t.Fatalf("standard terminal = %#v", standard.Terminal)
	}
	if transfers := standard.FileTransfers; transfers == nil || transfers.MaxConcurrent != 3 || len(transfers.ExcludePatterns) != 1 {
		t.Fatalf("standard transfers = %#v", standard.FileTransfers)
	}
	if len(migrated.Hosts) != 1 || migrated.Hosts[0].Tags[0] != "prod" {
		t.Fatalf("hosts = %#v, want them kept", migrated.Hosts)
	}
}

// schema 9 の組には、組に入れられなかったエンジンの設定が無い。schema 9 では標準の値が効いていた
// ので、プリセットへ移すときに標準の値で埋める。
func TestSchema9SetsBecomePresetsFilledFromTheStandardOne(t *testing.T) {
	migrated, err := migrateMetadataFromSchema9([]byte(metadataSchema9))
	if err != nil {
		t.Fatal(err)
	}

	if len(migrated.Presets) != 2 {
		t.Fatalf("presets = %#v, want the standard preset and the phone set", migrated.Presets)
	}
	phone := migrated.Presets[1]
	if phone.ID != "phone" || phone.Name != "スマホ" || phone.Terminal == nil || phone.Terminal.FontSize != 12 ||
		!phone.Terminal.ComposerEnterSends {
		t.Fatalf("phone preset = %#v", phone)
	}
	if phone.Terminal.MaxSessions != 8 || phone.Terminal.StartDirectory != "~/work" || phone.Terminal.LocalShellProfile != "zsh" {
		t.Fatalf("phone terminal = %#v, want the engine settings of the standard preset", phone.Terminal)
	}
	if phone.FileTransfers == nil || phone.FileTransfers.MaxConcurrent != 3 || phone.Appearance == nil || phone.Appearance.Accent != "sky" {
		t.Fatalf("phone preset = %#v, want the standard transfers and accent", phone)
	}
}

// 同期している2台がそれぞれ同じ schema 9 から移すと、同じバイト列になる。同期はそれを衝突と見なさない。
func TestMovingTheSameSchema9TwiceWritesTheSameBytes(t *testing.T) {
	first, err := migrateMetadataFromSchema9([]byte(metadataSchema9))
	if err != nil {
		t.Fatal(err)
	}
	second, err := migrateMetadataFromSchema9([]byte(metadataSchema9))
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := EncodeMetadata(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := EncodeMetadata(second)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("two migrations differ:\n%s\n---\n%s", firstBytes, secondBytes)
	}
}

// 同期で schema 9 のファイルが届いても、移した形で読める。
func TestASchema9FileIsReadInTheMovedForm(t *testing.T) {
	service, _ := newTerminalService(t)
	if err := service.metadata.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(metadataSchema9))

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}

	if stored.SchemaVersion != MetadataSchemaVersion || len(stored.Presets) != 2 {
		t.Fatalf("stored = %#v, want the moved form", stored)
	}
}
