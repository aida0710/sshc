package application

import (
	"errors"
	"testing"
)

func TestAnAccentIsStoredInTheMetadataAndReadBack(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "sky"}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Appearance == nil || stored.Appearance.Accent != "sky" {
		t.Fatalf("stored appearance = %+v, want accent sky", stored.Appearance)
	}
}

func TestChoosingNoAccentRemovesTheAppearanceSection(t *testing.T) {
	service, _ := newTerminalService(t)
	if _, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "pink"}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetAppearanceSettings(AppearanceSettings{}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Appearance != nil {
		t.Fatalf("stored appearance = %+v, want the section removed", stored.Appearance)
	}
}

func TestAnUnknownAccentIsRefusedAndNothingIsWritten(t *testing.T) {
	service, _ := newTerminalService(t)

	_, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "orange"})

	if !errors.Is(err, ErrMetadataAppearance) {
		t.Fatalf("err = %v, want ErrMetadataAppearance", err)
	}
	stored, _, loadErr := service.metadata.Load()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if stored.Appearance != nil {
		t.Fatalf("stored appearance = %+v, want nothing written", stored.Appearance)
	}
}

func TestTheAccentSurvivesEncodingTheMetadata(t *testing.T) {
	metadata := NewMetadata()
	metadata.Appearance = &AppearanceSettings{Accent: "mono"}

	encoded, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Appearance == nil || decoded.Appearance.Accent != "mono" {
		t.Fatalf("decoded appearance = %+v, want accent mono", decoded.Appearance)
	}
}

func TestACustomColourIsStoredAsTheAccent(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "#3fa86b"}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Appearance == nil || stored.Appearance.Accent != "#3fa86b" {
		t.Fatalf("stored appearance = %+v, want #3fa86b", stored.Appearance)
	}
}

func TestAnAccentThatIsNeitherAPresetNorAColourIsRefused(t *testing.T) {
	for _, accent := range []string{"custom", "green", "#3FA86B", "#3fa86", "#3fa86b80"} {
		service, _ := newTerminalService(t)

		_, err := service.SetAppearanceSettings(AppearanceSettings{Accent: accent})

		if !errors.Is(err, ErrMetadataAppearance) {
			t.Fatalf("SetAppearanceSettings(%q) err = %v, want ErrMetadataAppearance", accent, err)
		}
	}
}

// 同期で届いた metadata のさし色が読めなくても、metadata は読め、さし色だけが既定に戻る。
func TestAnUnreadableAccentFromAnotherMachineIsDroppedWhenRead(t *testing.T) {
	decoded, err := DecodeMetadata([]byte(`{"schemaVersion":9,"appearance":{"accent":"url(javascript:1)"},"hosts":[]}`))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v, want the metadata read", err)
	}
	if decoded.Appearance != nil {
		t.Fatalf("appearance = %+v, want it dropped", decoded.Appearance)
	}
}
