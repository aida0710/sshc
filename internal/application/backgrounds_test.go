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

func TestRenamingABackgroundMovesSavedReferencesInTheSameTransaction(t *testing.T) {
	service, workspace := newTerminalService(t)
	added, err := service.AddBackground("office", png("one"))
	if err != nil {
		t.Fatal(err)
	}
	metadata, precondition, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	metadata.EmbeddedTerminal = &EmbeddedTerminal{Appearance: &TerminalAppearance{Background: added.Name}}
	metadata.Hosts = []HostMetadata{{
		Identity:   HostIdentity{Path: "connections/work/host.conf", Alias: "host"},
		Appearance: &TerminalAppearance{Background: added.Name},
	}}
	change, err := service.metadata.Change(metadata, precondition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.manager.Commit(storage.Request{Operation: "fixture", Changes: []storage.Change{change}}); err != nil {
		t.Fatal(err)
	}

	renamed, err := service.RenameBackground(added.Name, "Night Sky.jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "night-sky.png" {
		t.Fatalf("renamed = %q, want a safe name with the detected extension", renamed.Name)
	}
	if _, _, err := service.BackgroundContents(added.Name); !errors.Is(err, ErrUnknownBackground) {
		t.Fatalf("old name still exists: %v", err)
	}
	contents, _, err := service.BackgroundContents(renamed.Name)
	if err != nil || !bytes.Equal(contents, png("one")) {
		t.Fatalf("renamed bytes = %q, %v", contents, err)
	}
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.EmbeddedTerminal.Appearance.Background; got != renamed.Name {
		t.Fatalf("overall background = %q", got)
	}
	if got := stored.Hosts[0].Appearance.Background; got != renamed.Name {
		t.Fatalf("host background = %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory), renamed.Name)); err != nil {
		t.Fatal(err)
	}
}

func TestRenamingABackgroundWillNotOverwriteAnotherImage(t *testing.T) {
	service, _ := newTerminalService(t)
	first, err := service.AddBackground("first", png("one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddBackground("second", png("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RenameBackground(first.Name, "second"); !errors.Is(err, ErrBackgroundAlreadyExists) {
		t.Fatalf("err = %v, want collision refused", err)
	}
}

func TestThereIsARoofOverWhatTheBackgroundsMayWeigh(t *testing.T) {
	service, _ := newTerminalService(t)
	if MaxBackgroundBytes != 1<<30 { t.Fatalf("absolute maximum = %d", MaxBackgroundBytes) }

	chunkSize := 1 << 20
	chunk := png(strings.Repeat("x", chunkSize-64))
	var lastErr error
	for round := 0; round < DefaultBackgroundCapacityMiB+2; round++ {
		if _, lastErr = service.AddBackground("wall", chunk); lastErr != nil {
			break
		}
	}
	if !errors.Is(lastErr, ErrBackgroundsFull) {
		t.Fatalf("err = %v, want the total to be capped", lastErr)
	}
}

func TestBackgroundCapacityCanBeChangedWithoutRewritingImages(t *testing.T) {
	service, workspace := newTerminalService(t)
	added, err := service.AddBackground("original", png("exact bytes"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.Root(), filepath.FromSlash(BackgroundsDirectory), added.Name)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetBackgroundCapacityMiB(64); err != nil {
		t.Fatal(err)
	}
	if got := service.BackgroundCapacityMiB(); got != 64 {
		t.Fatalf("capacity = %d", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("changing capacity rewrote the image")
	}
	if _, err := service.SetBackgroundCapacityMiB(0); !errors.Is(err, ErrBackgroundCapacity) {
		t.Fatalf("err = %v", err)
	}
}
