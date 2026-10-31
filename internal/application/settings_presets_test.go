package application

import (
	"errors"
	"fmt"
	"testing"
)

// editStandardPreset は、標準のプリセットを読み、change で変えて書く。
func editStandardPreset(t *testing.T, service *Service, change func(*SettingsPreset)) error {
	t.Helper()
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := stored.Presets[0]
	next := base
	change(&next)
	_, err = service.SetPreset(base, next)
	return err
}

func TestANewMetadataHasOnlyTheStandardPreset(t *testing.T) {
	service, _ := newTerminalService(t)

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}

	if len(stored.Presets) != 1 || stored.Presets[0].ID != DefaultPresetID || stored.Presets[0].Name != "" {
		t.Fatalf("presets = %+v, want only the standard preset", stored.Presets)
	}
}

func TestPresetsAreAddedRenamedAndRemovedAsAWholeList(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	phone := SettingsPreset{ID: "phone", Name: "スマホ", Terminal: &EmbeddedTerminal{FontSize: 12}}

	if _, err := service.SetPresets(stored.Presets, append(stored.Presets, phone)); err != nil {
		t.Fatal(err)
	}
	added, _, _ := service.metadata.Load()
	renamed := append([]SettingsPreset(nil), added.Presets...)
	renamed[1].Name = "Phone"
	if _, err := service.SetPresets(added.Presets, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetPresets(renamed, renamed[:1]); err != nil {
		t.Fatal(err)
	}

	final, _, _ := service.metadata.Load()
	if len(final.Presets) != 1 {
		t.Fatalf("presets = %+v, want the phone preset removed", final.Presets)
	}
}

func TestAListThatChangedSinceItWasReadIsRefused(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	stale := stored.Presets
	if _, err := service.SetPresets(stale, append(stale, SettingsPreset{ID: "phone", Name: "スマホ"})); err != nil {
		t.Fatal(err)
	}

	_, err := service.SetPresets(stale, append(stale, SettingsPreset{ID: "desk", Name: "PC"}))

	if !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("err = %v, want ErrSettingsChanged", err)
	}
}

func TestTheStandardPresetCannotBeRemovedRenamedOrMoved(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	phone := SettingsPreset{ID: "phone", Name: "スマホ"}

	for name, presets := range map[string][]SettingsPreset{
		"removed": {phone},
		"renamed": {{ID: DefaultPresetID, Name: "標準"}},
		"moved":   {phone, stored.Presets[0]},
	} {
		if _, err := service.SetPresets(stored.Presets, presets); !errors.Is(err, ErrPresets) {
			t.Fatalf("%s: err = %v, want ErrPresets", name, err)
		}
	}
}

func TestEditingOnePresetIsNotRefusedWhenAnotherPresetChanged(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	if _, err := service.SetPresets(stored.Presets, append(stored.Presets, SettingsPreset{ID: "phone", Name: "スマホ"})); err != nil {
		t.Fatal(err)
	}
	withPhone, _, _ := service.metadata.Load()
	phoneBase := withPhone.Presets[1]
	if err := editStandardPreset(t, service, func(preset *SettingsPreset) {
		preset.Terminal = &EmbeddedTerminal{FontSize: 15}
	}); err != nil {
		t.Fatal(err)
	}

	phone := phoneBase
	phone.Terminal = &EmbeddedTerminal{FontSize: 12}
	if _, err := service.SetPreset(phoneBase, phone); err != nil {
		t.Fatalf("editing the phone preset after the standard one changed: %v", err)
	}
}

func TestEditingAPresetThatChangedSinceItWasReadIsRefused(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	stale := stored.Presets[0]
	if err := editStandardPreset(t, service, func(preset *SettingsPreset) {
		preset.Terminal = &EmbeddedTerminal{FontSize: 15}
	}); err != nil {
		t.Fatal(err)
	}

	next := stale
	next.Terminal = &EmbeddedTerminal{FontSize: 20}
	if _, err := service.SetPreset(stale, next); !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("err = %v, want ErrSettingsChanged", err)
	}
}

func TestEditingAPresetCannotRenameIt(t *testing.T) {
	service, _ := newTerminalService(t)
	stored, _, _ := service.metadata.Load()
	if _, err := service.SetPresets(stored.Presets, append(stored.Presets, SettingsPreset{ID: "phone", Name: "スマホ"})); err != nil {
		t.Fatal(err)
	}
	withPhone, _, _ := service.metadata.Load()

	renamed := withPhone.Presets[1]
	renamed.Name = "Phone"
	if _, err := service.SetPreset(withPhone.Presets[1], renamed); !errors.Is(err, ErrSettingsChanged) {
		t.Fatalf("err = %v, want ErrSettingsChanged", err)
	}
}

// 前のバージョンが書いた欄の幅と Home の表示形式（今はブラウザに置く）が残っていても、プリセットはほかの値を
// 保って読め、そのまま保存できる。
func TestAPresetHoldingTheOldLayoutValuesKeepsItsOtherSettings(t *testing.T) {
	contents := fmt.Sprintf(`{"schemaVersion":%d,"presets":[{"id":"default","interface":{"touchMode":"touch","navigationWidth":300,"homeView":"list"}}]}`, MetadataSchemaVersion)

	metadata, err := DecodeMetadata([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	if got := metadata.Presets[0].Interface; got == nil || got.TouchMode != "touch" {
		t.Fatalf("interface = %+v, want the touch mode kept", got)
	}
	if err := ValidateMetadata(metadata); err != nil {
		t.Fatalf("the preset cannot be saved: %v", err)
	}
}

func TestInterfaceSettingsOutOfRangeAreRefused(t *testing.T) {
	for name, settings := range map[string]InterfaceSettings{
		"touch mode": {TouchMode: "auto"},
		"sound":      {NotificationSound: &NotificationSound{Sound: "siren", Volume: 50}},
		"volume":     {NotificationSound: &NotificationSound{Sound: "bell", Volume: 101}},
		"shortcuts":  {ShortcutPreset: "../x"},
	} {
		if err := validateInterfaceSettings(&settings); !errors.Is(err, ErrPresets) {
			t.Fatalf("%s: err = %v, want ErrPresets", name, err)
		}
	}
}
