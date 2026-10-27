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

func TestACustomAccentKeepsItsColour(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "custom", AccentColor: "#3fa86b"}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Appearance == nil || stored.Appearance.Accent != "custom" || stored.Appearance.AccentColor != "#3fa86b" {
		t.Fatalf("stored appearance = %+v, want custom #3fa86b", stored.Appearance)
	}
}

func TestACustomColourStaysWhileAPresetAccentIsChosen(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetAppearanceSettings(AppearanceSettings{Accent: "blue", AccentColor: "#3fa86b"}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Appearance == nil || stored.Appearance.Accent != "blue" || stored.Appearance.AccentColor != "#3fa86b" {
		t.Fatalf("stored appearance = %+v, want blue with #3fa86b kept", stored.Appearance)
	}
}

func TestACustomAccentWithoutAColourOrWithAnUnreadableOneIsRefused(t *testing.T) {
	for _, settings := range []AppearanceSettings{
		{Accent: "custom"},
		{Accent: "custom", AccentColor: "green"},
		{Accent: "custom", AccentColor: "#3FA86B"},
		{Accent: "custom", AccentColor: "#3fa86"},
		{Accent: "lavender", AccentColor: "#3fa86b80"},
	} {
		service, _ := newTerminalService(t)

		_, err := service.SetAppearanceSettings(settings)

		if !errors.Is(err, ErrMetadataAppearance) {
			t.Fatalf("SetAppearanceSettings(%+v) err = %v, want ErrMetadataAppearance", settings, err)
		}
	}
}
