package application

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func testKeyBarPreset() KeyBarPreset {
	return KeyBarPreset{
		ID:   "phone",
		Name: "スマホ用",
		Keys: []KeyBarKey{{Key: "Escape"}, {Key: "Control"}, {Key: "ArrowUp"}},
		MoreKeys: []KeyBarKey{
			{Key: "Alt"},
			{Label: "git st", Text: "git status", Enter: true},
			{Label: "|", Text: "|"},
		},
	}
}

func TestKeyBarPresetsPreserveOtherSettingsAndRejectStaleWrites(t *testing.T) {
	s, _ := newTerminalService(t)
	if err := setStandardTerminal(t, s, EmbeddedTerminal{FontSize: 18}); err != nil {
		t.Fatal(err)
	}
	first := []KeyBarPreset{testKeyBarPreset()}
	if _, err := s.SetKeyBarPresets(nil, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetKeyBarPresets(nil, nil); !errors.Is(err, ErrKeyBarConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Presets[0].Terminal.FontSize != 18 || !reflect.DeepEqual(stored.KeyBarPresets, first) {
		t.Fatal("settings lost")
	}
	if _, err := s.SetKeyBarPresets(first, nil); err != nil {
		t.Fatal(err)
	}
}

// 用意したキーは2つの段を通して1回ずつ。自作のキーは文字と内容の決まりに従い、用意したキーの名前と
// 一緒には持てない。
func TestKeyBarPresetsRejectKeysThatBreakTheRules(t *testing.T) {
	for name, change := range map[string]func(*KeyBarPreset){
		"the same key in both rows": func(p *KeyBarPreset) { p.MoreKeys = append(p.MoreKeys, KeyBarKey{Key: "Escape"}) },
		"an unknown key":            func(p *KeyBarPreset) { p.Keys[0] = KeyBarKey{Key: "Hyper"} },
		"a key with a label":        func(p *KeyBarPreset) { p.Keys[0].Label = "Esc" },
		"a custom key without text": func(p *KeyBarPreset) { p.MoreKeys[1].Text = "" },
		"a custom key sending ESC":  func(p *KeyBarPreset) { p.MoreKeys[1].Text = "\x1b[A" },
		"an empty key":              func(p *KeyBarPreset) { p.Keys[0] = KeyBarKey{} },
		"no list of more keys":      func(p *KeyBarPreset) { p.MoreKeys = nil },
		"the default id":            func(p *KeyBarPreset) { p.ID = "default" },
		"a blank name":              func(p *KeyBarPreset) { p.Name = " " },
		"too many keys": func(p *KeyBarPreset) {
			for len(p.Keys) <= 16 {
				p.Keys = append(p.Keys, KeyBarKey{Label: "x", Text: "x"})
			}
		},
	} {
		p := testKeyBarPreset()
		change(&p)
		if !errors.Is(validateKeyBarPresets([]KeyBarPreset{p}), ErrKeyBarPresets) {
			t.Errorf("%s was accepted", name)
		}
	}
	twice := testKeyBarPreset()
	twice.Keys = append(twice.Keys, twice.MoreKeys[1])
	if err := validateKeyBarPresets([]KeyBarPreset{twice}); err != nil {
		t.Errorf("the same custom key twice was refused: %v", err)
	}
}

// 後のビルドが足したキーは、そのキーだけを読み飛ばし、利用者が作った並びは残す。
func TestAKeyBarPresetWithAKeyFromALaterBuildKeepsTheRest(t *testing.T) {
	later := testKeyBarPreset()
	later.Keys = append(later.Keys, KeyBarKey{Key: "Hyper"})
	broken := testKeyBarPreset()
	broken.ID = "broken"
	broken.MoreKeys = append(broken.MoreKeys, KeyBarKey{Label: "bad", Text: "\x03"})
	metadata := NewMetadata()
	metadata.KeyBarPresets = []KeyBarPreset{later, broken}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}

	read, err := DecodeMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.KeyBarPresets) != 1 || read.KeyBarPresets[0].ID != later.ID {
		t.Fatalf("presets = %+v, want only the one the build can read", read.KeyBarPresets)
	}
	if !reflect.DeepEqual(read.KeyBarPresets[0].Keys, testKeyBarPreset().Keys) {
		t.Fatalf("keys = %+v, want the keys the user placed without the unknown one", read.KeyBarPresets[0].Keys)
	}
	if _, err := EncodeMetadata(read); err != nil {
		t.Fatalf("saving after the read refused: %v", err)
	}
}

func TestASettingsPresetMayChooseAKeyBarPreset(t *testing.T) {
	if err := validateInterfaceSettings(&InterfaceSettings{KeyBarPreset: "phone"}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(validateInterfaceSettings(&InterfaceSettings{KeyBarPreset: "not an id"}), ErrPresets) {
		t.Fatal("a malformed key bar preset id was accepted")
	}
}
