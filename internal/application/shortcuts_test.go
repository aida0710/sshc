package application

import (
	"errors"
	"reflect"
	"testing"
)

func testShortcutPreset() ShortcutPreset {
	bindings := map[string][]string{}
	for _, a := range shortcutActions {
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
