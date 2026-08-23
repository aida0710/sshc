package application

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

func png(payload string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte(payload)...)
}

func TestTheServerNamesTheFileItWrites(t *testing.T) {
	service, workspace := newTerminalService(t)

	for _, probe := range []struct{ suggested, want string }{
		{"../../../etc/passwd", "etc-passwd.png"},
		{"/absolute/path.png", "absolute-path.png"},
		{".hidden", "hidden.png"},
		{"Office Wall.JPG", "office-wall.png"},
		{"..", ""},
		{"", ""},
	} {
		background, err := service.AddBackground(probe.suggested, png(probe.suggested+"payload"))
		if err != nil {
			t.Fatalf("%q: %v", probe.suggested, err)
		}
		if probe.want != "" && background.Name != probe.want {
			t.Fatalf("%q became %q, want %q", probe.suggested, background.Name, probe.want)
		}
		written := filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory), background.Name)
		if _, err := os.Stat(written); err != nil {
			t.Fatalf("%q was written somewhere else: %v", probe.suggested, err)
		}
		if strings.ContainsAny(background.Name, "/\\") || strings.HasPrefix(background.Name, ".") {
			t.Fatalf("%q became an unsafe name %q", probe.suggested, background.Name)
		}
	}
}

func TestOnlyBytesThatLookLikeAnImageAreStored(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.AddBackground("payload.png", []byte("<html><script>alert(1)</script>")); !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want it refused as not an image", err)
	}
	if _, err := service.AddBackground("art.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); !errors.Is(err, ErrNotAnImage) {
		t.Fatalf("err = %v, want svg refused", err)
	}
	background, err := service.AddBackground("photo.txt", png("real"))
	if err != nil {
		t.Fatal(err)
	}
	if background.Name != "photo.png" || background.Type != "image/png" {
		t.Fatalf("background = %#v, want the extension to come from the bytes", background)
	}
}

func TestBackgroundsRoundTripAndCanBeRemoved(t *testing.T) {
	service, _ := newTerminalService(t)

	added, err := service.AddBackground("office", png("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	contents, mediaType, err := service.BackgroundContents(added.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, png("bytes")) || mediaType != "image/png" {
		t.Fatalf("read back %d bytes as %q", len(contents), mediaType)
	}

	if err := service.RemoveBackground(added.Name); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.BackgroundContents(added.Name); !errors.Is(err, ErrUnknownBackground) {
		t.Fatalf("err = %v, want it gone", err)
	}
}

func TestReadingRefusesNamesThatWereNeverStored(t *testing.T) {
	service, _ := newTerminalService(t)
	if _, err := service.AddBackground("office", png("bytes")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"../metadata.json", "../../config", "office", "", "office.png/../../x"} {
		if _, _, err := service.BackgroundContents(name); !errors.Is(err, ErrUnknownBackground) {
			t.Fatalf("%q was readable: %v", name, err)
		}
	}
}

func TestTwoImagesWithTheSameNameBothSurvive(t *testing.T) {
	service, _ := newTerminalService(t)

	first, err := service.AddBackground("wall", png("one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AddBackground("wall", png("two"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Name == second.Name {
		t.Fatalf("both were called %q, one overwrote the other", first.Name)
	}
	if _, _, err := service.BackgroundContents(first.Name); err != nil {
		t.Fatalf("the first one is gone: %v", err)
	}
}

func TestThereIsARoofOverWhatTheBackgroundsMayWeigh(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.AddBackground("huge", png(strings.Repeat("x", MaxBackgroundBytes))); !errors.Is(err, ErrBackgroundTooLarge) {
		t.Fatalf("err = %v, want a single oversized image refused", err)
	}

	if MaxBackgroundBytes > storage.MaxFileSize {
		t.Fatalf("MaxBackgroundBytes = %d, larger than the layer will read back (%d)", MaxBackgroundBytes, storage.MaxFileSize)
	}

	chunk := png(strings.Repeat("x", MaxBackgroundBytes-64))
	var lastErr error
	for round := 0; round < (MaxBackgroundsBytes/MaxBackgroundBytes)+2; round++ {
		if _, lastErr = service.AddBackground("wall", chunk); lastErr != nil {
			break
		}
	}
	if !errors.Is(lastErr, ErrBackgroundsFull) {
		t.Fatalf("err = %v, want the total to be capped", lastErr)
	}
}
