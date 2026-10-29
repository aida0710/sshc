package application

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func phonePreset() TerminalPreset {
	enter := true
	return TerminalPreset{ID: "phone", Name: "スマホ", Settings: EmbeddedTerminal{FontSize: 12, ComposerEnterSends: enter, WebGL: new(bool)}}
}

func TestTerminalPresetsKeepTheStandardSettingsAndRefuseAStaleList(t *testing.T) {
	s, _ := newTerminalService(t)
	if _, err := s.SetTerminalSettings(TerminalSettings{FontSize: 18, MaxSessions: 20}); err != nil {
		t.Fatal(err)
	}
	first := []TerminalPreset{phonePreset()}
	if _, err := s.SetTerminalPresets(nil, first); err != nil {
		t.Fatal(err)
	}
	// A screen that read the list before the phone preset was added must not wipe it.
	if _, err := s.SetTerminalPresets(nil, nil); !errors.Is(err, ErrTerminalPresetConflict) {
		t.Fatalf("stale write: %v, want a conflict", err)
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.EmbeddedTerminal.FontSize != 18 || stored.EmbeddedTerminal.MaxSessions != 20 || !reflect.DeepEqual(stored.TerminalPresets, first) {
		t.Fatalf("stored = %+v %+v, want the standard settings and the preset both kept", stored.EmbeddedTerminal, stored.TerminalPresets)
	}
	if _, err := s.SetTerminalPresets(first, nil); err != nil {
		t.Fatal(err)
	}
	stored, _, err = s.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TerminalPresets != nil {
		t.Fatalf("presets = %+v, want none left", stored.TerminalPresets)
	}
}

func TestATerminalPresetHoldsOnlyWhatTheBrowserApplies(t *testing.T) {
	reconnect := 2
	for name, engineWide := range map[string]EmbeddedTerminal{
		"session limit":   {MaxSessions: 10},
		"replay buffer":   {ScrollbackBytes: 65536},
		"connection log":  {Verbosity: 1},
		"reconnects":      {Reconnect: &reconnect},
		"OSC 52":          {OSC52: true},
		"local shell":     {LocalShellProfile: "zsh"},
		"start directory": {StartDirectory: "~/work"},
	} {
		t.Run(name, func(t *testing.T) {
			preset := TerminalPreset{ID: "pc", Name: "PC", Settings: engineWide}
			if err := validateTerminalPresets([]TerminalPreset{preset}); !errors.Is(err, ErrTerminalPresets) {
				t.Fatalf("validate = %v, want it refused", err)
			}
		})
	}
}

func TestTerminalPresetsRefuseBadNamesIdsAndValues(t *testing.T) {
	for name, preset := range map[string]TerminalPreset{
		"the standard preset's id": {ID: "default", Name: "PC"},
		"an id with a space":       {ID: "my pc", Name: "PC"},
		"an empty name":            {ID: "pc", Name: "  "},
		"a name with a newline":    {ID: "pc", Name: "P\nC"},
		"a name too long":          {ID: "pc", Name: strings.Repeat("あ", MaxTerminalPresetNameLength+1)},
		"a font too large":         {ID: "pc", Name: "PC", Settings: EmbeddedTerminal{FontSize: 99}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateTerminalPresets([]TerminalPreset{preset}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if err := validateTerminalPresets([]TerminalPreset{phonePreset(), phonePreset()}); err == nil {
		t.Fatal("two presets with one id accepted")
	}
	many := make([]TerminalPreset, MaxTerminalPresets+1)
	for index := range many {
		many[index] = TerminalPreset{ID: "p" + strings.Repeat("x", index), Name: "P"}
	}
	if err := validateTerminalPresets(many); err == nil {
		t.Fatal("more presets than the limit accepted")
	}
}

func TestMetadataDropsATerminalPresetThisBuildCannotUse(t *testing.T) {
	decoded, err := DecodeMetadata([]byte(`{"schemaVersion":` + strconv.Itoa(MetadataSchemaVersion) + `,"terminalPresets":[` +
		`{"id":"phone","name":"スマホ","settings":{"fontSize":12}},` +
		`{"id":"broken","name":"壊れた組","settings":{"maxSessions":10}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.TerminalPresets) != 1 || decoded.TerminalPresets[0].ID != "phone" {
		t.Fatalf("presets = %+v, want only the phone preset", decoded.TerminalPresets)
	}
}
