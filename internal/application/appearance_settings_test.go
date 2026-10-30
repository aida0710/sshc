package application

import (
	"errors"
	"testing"
)

func TestAnAccentAndAThemeAreStoredInThePresetAndReadBack(t *testing.T) {
	service, _ := newTerminalService(t)

	if err := editStandardPreset(t, service, func(preset *SettingsPreset) {
		preset.Appearance = &AppearanceSettings{Accent: "sky", Theme: "light"}
	}); err != nil {
		t.Fatal(err)
	}

	appearance := service.EffectiveSettings().Preset.Appearance
	if appearance == nil || appearance.Accent != "sky" || appearance.Theme != "light" {
		t.Fatalf("appearance = %+v, want accent sky and theme light", appearance)
	}
}

func TestACustomColourIsStoredAsTheAccent(t *testing.T) {
	service, _ := newTerminalService(t)

	if err := editStandardPreset(t, service, func(preset *SettingsPreset) {
		preset.Appearance = &AppearanceSettings{Accent: "#3fa86b"}
	}); err != nil {
		t.Fatal(err)
	}

	if appearance := service.EffectiveSettings().Preset.Appearance; appearance == nil || appearance.Accent != "#3fa86b" {
		t.Fatalf("appearance = %+v, want #3fa86b", appearance)
	}
}

func TestAnAccentThatIsNeitherAPresetNorAColourIsRefused(t *testing.T) {
	for _, accent := range []string{"custom", "green", "#3FA86B", "#3fa86", "#3fa86b80"} {
		service, _ := newTerminalService(t)

		err := editStandardPreset(t, service, func(preset *SettingsPreset) {
			preset.Appearance = &AppearanceSettings{Accent: accent}
		})

		if !errors.Is(err, ErrMetadataAppearance) {
			t.Fatalf("accent %q: err = %v, want ErrMetadataAppearance", accent, err)
		}
	}
}

func TestAThemeNameThatIsNotAPlainWordIsRefused(t *testing.T) {
	for _, theme := range []string{"Dark", "dark mode", "../dark", ""} {
		err := validateAppearanceSettings(&AppearanceSettings{Theme: theme})
		if theme == "" {
			if err != nil {
				t.Fatalf("an empty theme was refused: %v", err)
			}
			continue
		}
		if !errors.Is(err, ErrMetadataAppearance) {
			t.Fatalf("theme %q: err = %v, want ErrMetadataAppearance", theme, err)
		}
	}
}

// 同期で届いた metadata のプリセットが読めなくても、metadata は読め、そのプリセットだけが捨てられる。
func TestAnUnreadablePresetFromAnotherMachineIsDroppedWhenRead(t *testing.T) {
	decoded, err := DecodeMetadata([]byte(`{"schemaVersion":10,"presets":[{"id":"default","appearance":{"accent":"url(javascript:1)"}},{"id":"phone","name":"スマホ"}],"hosts":[]}`))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v, want the metadata read", err)
	}
	if len(decoded.Presets) != 2 || decoded.Presets[0].Appearance != nil || decoded.Presets[1].ID != "phone" {
		t.Fatalf("presets = %+v, want the standard preset reset and the phone preset kept", decoded.Presets)
	}
}
