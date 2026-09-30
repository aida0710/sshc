package application

import (
	"errors"
	"reflect"
	"testing"

	"sshc/internal/validate"
)

func testShortcutPreset() ShortcutPreset {
	bindings := map[string][]string{}
	for _, a := range validate.ShortcutActions {
		bindings[a] = []string{}
	}
	bindings["palette"] = []string{"Ctrl+K", "Meta+K"}
	return ShortcutPreset{ID: "test-preset", Name: "Work", Bindings: bindings}
}
func TestShortcutPresetsPreserveOtherSettingsAndRejectStaleWrites(t *testing.T) {
	s, _ := newTerminalService(t)
	if _, err := s.SetTerminalSettings(TerminalSettings{FontSize: 18}); err != nil {
		t.Fatal(err)
	}
	first := []ShortcutPreset{testShortcutPreset()}
	if _, err := s.SetShortcutPresets(nil, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetShortcutPresets(nil, nil); !errors.Is(err, ErrShortcutConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.EmbeddedTerminal.FontSize != 18 || !reflect.DeepEqual(stored.ShortcutPresets, first) {
		t.Fatal("settings lost")
	}
	if _, err := s.SetShortcutPresets(first, nil); err != nil {
		t.Fatal(err)
	}
}
func TestShortcutPresetsRejectConflictsAndMalformedBindings(t *testing.T) {
	for _, invalid := range []string{"Ctrl+K", "Shift+A", "Ctrl+Alt+Alt+X", "A", "Ctrl+f"} {
		p := testShortcutPreset()
		p.Bindings["home"] = []string{invalid}
		if !errors.Is(validateShortcutPresets([]ShortcutPreset{p}), ErrShortcutPresets) {
			t.Errorf("accepted %q", invalid)
		}
	}
	p := testShortcutPreset()
	delete(p.Bindings, "copy")
	if validateShortcutPresets([]ShortcutPreset{p}) == nil {
		t.Fatal("missing action accepted")
	}
}

// 画面はファンクションキーを修飾なしでも Shift などの修飾付きでも記録する。
// 保存がそれを断ると、利用者は理由の分からない保存の失敗を見る。
func TestShortcutPresetsAcceptFunctionKeysWithAnyModifiers(t *testing.T) {
	p := testShortcutPreset()
	p.Bindings["home"] = []string{"F2", "Shift+F2", "Ctrl+Shift+F13"}
	p.Bindings["sftp"] = []string{"F24"}
	if err := validateShortcutPresets([]ShortcutPreset{p}); err != nil {
		t.Fatalf("function keys were refused: %v", err)
	}
}
