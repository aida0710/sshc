package application

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/platform"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

func newTerminalService(t *testing.T) (*Service, *storage.Workspace) {
	t.Helper()
	workspace := newTestWorkspace(t)
	return NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader)), workspace
}

func TestTheStartDirectoryDefaultsToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)

	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home %q", got, workspace.Home())
	}
}

func TestTheStartDirectoryKeepsTheTildeAndResolvesItWhenRead(t *testing.T) {
	service, workspace := newTerminalService(t)
	work := filepath.Join(workspace.Home(), "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: "~/work"}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TerminalStartDirectory() != "~/work" {
		t.Fatalf("stored = %q, want the tilde kept", stored.TerminalStartDirectory())
	}
	if got := service.TerminalStartDirectory(); got != work {
		t.Fatalf("start directory = %q, want %q", got, work)
	}
}

func TestTheStartDirectoryIsRefusedWhenItCannotBeUsed(t *testing.T) {
	service, workspace := newTerminalService(t)
	file := filepath.Join(workspace.Home(), "notes.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		given string
		want  error
	}{
		{name: "relative", given: "work", want: platform.ErrDirectoryRelative},
		{name: "another user", given: "~someone", want: platform.ErrDirectoryUser},
		{name: "missing", given: "~/nowhere", want: ErrStartDirectoryMissing},
		{name: "a file", given: file, want: ErrStartDirectoryNotADirectory},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: test.given}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if got := service.TerminalStartDirectory(); got != workspace.Home() {
				t.Fatalf("the refusal changed the start directory to %q", got)
			}
		})
	}
}

func TestSavingDoesNotWriteSettingsNobodyChose(t *testing.T) {
	service, workspace := newTerminalService(t)

	for round := 0; round < 2; round++ {
		if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: "~"}); err != nil {
			t.Fatal(err)
		}
	}

	contents, err := os.ReadFile(filepath.Join(workspace.Root(), "sshc", MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"maxSessions", "scrollbackBytes"} {
		if strings.Contains(string(contents), unwanted) {
			t.Fatalf("saving the start directory wrote %q into metadata:\n%s", unwanted, contents)
		}
	}
}

func TestTheLimitsRoundTripAndCanBeCleared(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetTerminalSettings(TerminalSettings{
		MaxSessions: 4, ScrollbackBytes: 32768,
	}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalSettings(); got.MaxSessions != 4 || got.ScrollbackBytes != 32768 {
		t.Fatalf("settings = %#v", got)
	}
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if limits := stored.TerminalLimits(); limits.MaxSessions != 4 || limits.Scrollback != 32768 {
		t.Fatalf("limits = %#v, want the stored numbers to reach the terminal", limits)
	}

	if _, err := service.SetTerminalSettings(TerminalSettings{}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalSettings(); got != (TerminalSettings{}) {
		t.Fatalf("settings after clearing = %#v", got)
	}
}

func TestTheClipboardChoicesRoundTripAndCanBeCleared(t *testing.T) {
	service, _ := newTerminalService(t)
	off := false

	if _, err := service.SetTerminalSettings(TerminalSettings{
		CopyOnSelect: &off, RightClickPaste: &off,
	}); err != nil {
		t.Fatal(err)
	}
	got := service.TerminalSettings()
	if got.CopyOnSelect == nil || *got.CopyOnSelect || got.RightClickPaste == nil || *got.RightClickPaste {
		t.Fatalf("settings = %#v, want both choices explicitly off", got)
	}

	if _, err := service.SetTerminalSettings(TerminalSettings{}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalSettings(); got != (TerminalSettings{}) {
		t.Fatalf("settings after clearing = %#v", got)
	}
}

func TestTheLimitsAreRefusedOutsideTheirRange(t *testing.T) {
	service, _ := newTerminalService(t)

	for name, settings := range map[string]TerminalSettings{
		"too many sessions":    {MaxSessions: 9999},
		"negative sessions":    {MaxSessions: -1},
		"scrollback too small": {ScrollbackBytes: 1},
		"scrollback too large": {ScrollbackBytes: 99 << 20},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.SetTerminalSettings(settings); !errors.Is(err, ErrMetadataTerminal) {
				t.Fatalf("error = %v, want ErrMetadataTerminal", err)
			}
			if got := service.TerminalSettings(); got != (TerminalSettings{}) {
				t.Fatalf("the refusal wrote %#v", got)
			}
		})
	}
}

func TestAStartDirectoryThatDisappearedFallsBackToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)
	work := filepath.Join(workspace.Home(), "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: "~/work"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}

	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home %q", got, workspace.Home())
	}
}

func TestClearingTheStartDirectoryReturnsToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)
	if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: "~"}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: ""}); err != nil {
		t.Fatal(err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TerminalStartDirectory() != "" {
		t.Fatalf("stored = %q, want it cleared", stored.TerminalStartDirectory())
	}
	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home", got)
	}
}

func TestTheAppearanceRoundTripsAndCanBeCleared(t *testing.T) {
	service, _ := newTerminalService(t)

	if _, err := service.SetTerminalSettings(TerminalSettings{
		Appearance: TerminalAppearance{Palette: "dracula"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalSettings().Appearance.Palette; got != "dracula" {
		t.Fatalf("palette = %q, want it read back", got)
	}

	if _, err := service.SetTerminalSettings(TerminalSettings{}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalSettings().Appearance; !got.Empty() {
		t.Fatalf("appearance = %#v, want it cleared", got)
	}
}

func TestSavingDoesNotWriteAnAppearanceNobodyChose(t *testing.T) {
	service, workspace := newTerminalService(t)

	if _, err := service.SetTerminalSettings(TerminalSettings{StartDirectory: "~"}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(workspace.Root(), "sshc", MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "appearance") {
		t.Fatalf("saving the start directory wrote an appearance into metadata:\n%s", contents)
	}
}

func TestChoosingNoReconnectSurvivesTheRoundTrip(t *testing.T) {
	service, _ := newTerminalService(t)
	never := 0

	if _, err := service.SetTerminalSettings(TerminalSettings{Reconnect: &never}); err != nil {
		t.Fatal(err)
	}

	settings := service.TerminalSettings()
	if settings.Reconnect == nil {
		t.Fatal("繋ぎ直さないという選択が、書かれていない状態として読まれた")
	}
	if *settings.Reconnect != 0 {
		t.Fatalf("Reconnect = %d, want 0", *settings.Reconnect)
	}
	if attempts := service.TerminalReconnects(); attempts != 0 {
		t.Fatalf("TerminalReconnects = %d, want 0", attempts)
	}
}

func TestAnUnsetReconnectFallsBackToTheDefault(t *testing.T) {
	service, _ := newTerminalService(t)
	if attempts := service.TerminalReconnects(); attempts != terminal.MaxReconnects {
		t.Fatalf("TerminalReconnects = %d, want %d", attempts, terminal.MaxReconnects)
	}

	tooMany := 99
	if _, err := service.SetTerminalSettings(TerminalSettings{Reconnect: &tooMany}); err != nil {
		t.Fatal(err)
	}
	if attempts := service.TerminalReconnects(); attempts != terminal.MaxReconnects {
		t.Fatalf("out of range = %d, want %d", attempts, terminal.MaxReconnects)
	}
}
