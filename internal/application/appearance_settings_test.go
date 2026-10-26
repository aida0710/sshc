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
