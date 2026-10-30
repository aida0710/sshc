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

// setStandardTerminal は、標準のプリセットのターミナルの設定を置き換える。
func setStandardTerminal(t *testing.T, service *Service, settings EmbeddedTerminal) error {
	t.Helper()
	return editStandardPreset(t, service, func(preset *SettingsPreset) { preset.Terminal = storedSection(settings) })
}

// setGlobalTerminal は、全体の設定のうちターミナルのものを置き換える。
func setGlobalTerminal(t *testing.T, service *Service, settings GlobalTerminalSettings) error {
	t.Helper()
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := globalSettingsOf(stored)
	next := base
	next.Terminal = storedSection(settings)
	_, err = service.SetGlobalSettings(base, next)
	return err
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

	if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~/work"}); err != nil {
		t.Fatal(err)
	}

	if stored := service.terminalPresetSettings().StartDirectory; stored != "~/work" {
		t.Fatalf("stored = %q, want the tilde kept", stored)
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
			if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: test.given}); !errors.Is(err, test.want) {
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
		if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~"}); err != nil {
			t.Fatal(err)
		}
	}

	contents, err := os.ReadFile(filepath.Join(workspace.Root(), "sshc", MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"maxSessions", "scrollbackBytes", "appearance"} {
		if strings.Contains(string(contents), unwanted) {
			t.Fatalf("saving the start directory wrote %q into metadata:\n%s", unwanted, contents)
		}
	}
}

func TestTheLimitsRoundTripAndCanBeCleared(t *testing.T) {
	service, _ := newTerminalService(t)

	if err := setStandardTerminal(t, service, EmbeddedTerminal{MaxSessions: 4, ScrollbackBytes: 32768}); err != nil {
		t.Fatal(err)
	}
	if limits := service.TerminalLimits(); limits.MaxSessions != 4 || limits.Scrollback != 32768 {
		t.Fatalf("limits = %#v, want the stored numbers to reach the terminal", limits)
	}

	if err := setStandardTerminal(t, service, EmbeddedTerminal{}); err != nil {
		t.Fatal(err)
	}
	if limits := service.TerminalLimits(); limits != terminal.DefaultLimits() {
		t.Fatalf("limits after clearing = %#v", limits)
	}
}

func TestTerminalChoicesRoundTripThroughThePreset(t *testing.T) {
	service, _ := newTerminalService(t)
	off := false
	want := EmbeddedTerminal{
		CopyOnSelect:           &off,
		RightClickPaste:        &off,
		BrowserScrollbackLines: 12000,
		RetainedTerminals:      20,
		WebGL:                  &off,
		JISYenBackslash:        true,
		ComposerEnterSends:     true,
		Ligatures:              true,
		StatusLine:             &off,
		CommandMarks:           &off,
		StickyCommand:          &off,
		CommandScrollbarMarks:  &off,
		LocalShellProfile:      "fish",
		Appearance:             &TerminalAppearance{Palette: "dracula"},
	}
	if err := setStandardTerminal(t, service, want); err != nil {
		t.Fatal(err)
	}

	if got := service.terminalPresetSettings(); !sameSettings(got, want) {
		t.Fatalf("settings = %#v, want %#v", got, want)
	}
	if got := service.TerminalLocalShellProfile(); got != "fish" {
		t.Fatalf("local shell profile = %q, want fish", got)
	}
}

func TestTheLimitsAreRefusedOutsideTheirRange(t *testing.T) {
	service, _ := newTerminalService(t)
	tint := 500

	for name, settings := range map[string]EmbeddedTerminal{
		"too many sessions":       {MaxSessions: 9999},
		"negative sessions":       {MaxSessions: -1},
		"scrollback too small":    {ScrollbackBytes: 1},
		"scrollback too large":    {ScrollbackBytes: 99 << 20},
		"browser lines too small": {BrowserScrollbackLines: 999},
		"browser lines too large": {BrowserScrollbackLines: 100001},
		"no terminal kept":        {RetainedTerminals: -1},
		"too many terminals kept": {RetainedTerminals: 100},
		"invalid shell profile":   {LocalShellProfile: "../../bin/sh"},
		"backgroundTint":          {Appearance: &TerminalAppearance{BackgroundTint: &tint}},
		"palette":                 {Appearance: &TerminalAppearance{Palette: strings.Repeat("p", 65)}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := setStandardTerminal(t, service, settings); !errors.Is(err, ErrMetadataTerminal) {
				t.Fatalf("error = %v, want ErrMetadataTerminal", err)
			}
			if got := service.terminalPresetSettings(); !sameSettings(got, EmbeddedTerminal{}) {
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
	if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~/work"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}

	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home %q", got, workspace.Home())
	}
}

func TestAnUnchangedMissingStartDirectoryDoesNotBlockOtherTerminalSettings(t *testing.T) {
	service, workspace := newTerminalService(t)
	work := filepath.Join(workspace.Home(), "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~/work"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}

	if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~/work", FontSize: 16}); err != nil {
		t.Fatalf("saving an unrelated setting with the unchanged directory = %v", err)
	}
	if got := service.terminalPresetSettings(); got.StartDirectory != "~/work" || got.FontSize != 16 {
		t.Fatalf("settings = %#v", got)
	}
}

func TestClearingTheStartDirectoryReturnsToTheHome(t *testing.T) {
	service, workspace := newTerminalService(t)
	if err := setStandardTerminal(t, service, EmbeddedTerminal{StartDirectory: "~"}); err != nil {
		t.Fatal(err)
	}

	if err := setStandardTerminal(t, service, EmbeddedTerminal{}); err != nil {
		t.Fatal(err)
	}

	if stored := service.terminalPresetSettings().StartDirectory; stored != "" {
		t.Fatalf("stored = %q, want it cleared", stored)
	}
	if got := service.TerminalStartDirectory(); got != workspace.Home() {
		t.Fatalf("start directory = %q, want the home", got)
	}
}

func TestChoosingNoReconnectSurvivesTheRoundTrip(t *testing.T) {
	service, _ := newTerminalService(t)
	never := 0

	if err := setGlobalTerminal(t, service, GlobalTerminalSettings{Reconnect: &never}); err != nil {
		t.Fatal(err)
	}

	if attempts := service.TerminalReconnects(); attempts != 0 {
		t.Fatalf("TerminalReconnects = %d, want 0 (繋ぎ直さないという選択が、書かれていない状態として読まれた)", attempts)
	}
}

func TestAnUnsetReconnectFallsBackToTheDefault(t *testing.T) {
	service, _ := newTerminalService(t)
	if attempts := service.TerminalReconnects(); attempts != terminal.MaxReconnects {
		t.Fatalf("TerminalReconnects = %d, want %d", attempts, terminal.MaxReconnects)
	}

	tooMany := 99
	if err := setGlobalTerminal(t, service, GlobalTerminalSettings{Reconnect: &tooMany}); !errors.Is(err, ErrMetadataTerminal) {
		t.Fatalf("reconnect 99 = %v, want ErrMetadataTerminal", err)
	}
	if attempts := service.TerminalReconnects(); attempts != terminal.MaxReconnects {
		t.Fatalf("after a refused write = %d, want %d", attempts, terminal.MaxReconnects)
	}
}

func TestTheConnectionLogLevelIsRefusedOutsideItsRange(t *testing.T) {
	service, _ := newTerminalService(t)

	if err := setGlobalTerminal(t, service, GlobalTerminalSettings{Verbosity: 9}); !errors.Is(err, ErrMetadataTerminal) {
		t.Fatalf("verbosity 9 = %v, want ErrMetadataTerminal", err)
	}
	if err := setGlobalTerminal(t, service, GlobalTerminalSettings{Verbosity: 2}); err != nil {
		t.Fatal(err)
	}
	if got := service.TerminalVerbosity(); got != 2 {
		t.Fatalf("verbosity = %d, want 2", got)
	}
}
