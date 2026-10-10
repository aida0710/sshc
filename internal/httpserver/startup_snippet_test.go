package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"sshc/internal/api"
	"sshc/internal/secret"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
)

// startupFixture は、alias「production」に起動スニペットを割り当てた snippets.Service と、
// 今の接続先の binding を差し替える口である。
type startupFixture struct {
	service *snippets.Service
	// destination は、resolver が今返す接続先の binding である。
	destination string
}

// newStartupFixture は、接続先の binding が「assigned」のときに起動スニペットを割り当てる。
func newStartupFixture(t *testing.T) *startupFixture {
	t.Helper()
	fixture := &startupFixture{destination: "assigned"}
	fixture.service = snippets.NewService(snippets.Options{
		Repository: &commandRepository{},
		Resolve: func(alias string) (snippets.Resolution, error) {
			return snippets.Resolution{
				Target:  snippets.Target{Alias: alias, HostName: "203.0.113.10"},
				Binding: fixture.destination,
			}, nil
		},
		Now: time.Now,
	})
	snippet, err := fixture.service.Create(snippets.Draft{Name: "app", Command: "cd /srv/app"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.SetStartup("production", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// recordingLogger は、Debug 以上のすべての記録を文字列として残す logger である。
func recordingLogger() (*slog.Logger, *bytes.Buffer) {
	var records bytes.Buffer
	return slog.New(slog.NewTextHandler(&records, &slog.HandlerOptions{Level: slog.LevelDebug})), &records
}

func TestAStartupSnippetIsSentWhileTheDestinationIsUnchanged(t *testing.T) {
	fixture := newStartupFixture(t)
	logger, records := recordingLogger()

	got := prepareStartupSnippet(fixture.service, logger, "production")

	if got != (StartupSnippet{Command: "cd /srv/app"}) {
		t.Fatalf("startup = %#v", got)
	}
	if records.Len() != 0 {
		t.Errorf("a sent startup snippet was logged: %s", records.String())
	}
}

// 割り当てた後で接続先が変わったら送らず、そのことをターミナルでも知らせる。engine の
// 記録だけでは、利用者は起動スニペットが止まったことに気付けない。
func TestAStartupSnippetForAChangedDestinationIsHeldBackAndTheTerminalIsTold(t *testing.T) {
	fixture := newStartupFixture(t)
	fixture.destination = "moved"
	logger, records := recordingLogger()

	got := prepareStartupSnippet(fixture.service, logger, "production")

	if got != (StartupSnippet{Notice: startupDestinationChangedNotice}) {
		t.Fatalf("startup = %#v", got)
	}
	if !strings.Contains(records.String(), "level=WARN") {
		t.Errorf("the held-back startup snippet was not logged as a warning: %s", records.String())
	}
}

// lockedLibrary は、Vault がロックされていてスニペットの文書を読めない状態である。
type lockedLibrary struct{}

func (lockedLibrary) Load() (snippets.Library, error)            { return snippets.Library{}, secret.ErrLocked }
func (lockedLibrary) Save(snippets.Library) error                { return secret.ErrLocked }
func (lockedLibrary) Mutate(func(*snippets.Library) error) error { return secret.ErrLocked }

// ロック中は割り当てがあるかも分からない。自動ロックの後に接続するのは普通の使い方で、
// 割り当てていないホストでも毎回ここを通るので、送らなかったと警告しない。
func TestALockedLibraryDoesNotWarnAboutAStartupSnippetOnEveryConnection(t *testing.T) {
	service := snippets.NewService(snippets.Options{Repository: lockedLibrary{}})
	logger, records := recordingLogger()

	got := prepareStartupSnippet(service, logger, "production")

	if got != (StartupSnippet{}) {
		t.Fatalf("startup = %#v, want nothing to send or say", got)
	}
	if strings.Contains(records.String(), "level=WARN") {
		t.Errorf("a locked library was logged as a missed startup snippet: %s", records.String())
	}
	if !strings.Contains(records.String(), "level=DEBUG") {
		t.Errorf("a locked library left no trace: %s", records.String())
	}
}

// 送らなかったことの案内は、コマンドの代わりに接続のたびの知らせとして渡す。Ready を
// 待ってからターミナルへ書くのは terminal.Session である（sendStartup）。
func TestAHeldBackStartupSnippetIsHandedOverAsANoticeWithoutACommand(t *testing.T) {
	handlers := TerminalHandlers{
		Connect: func(context.Context, string, terminal.Size) (terminal.Process, error) {
			return newScriptedPTY(), nil
		},
		Startup: func(string) StartupSnippet {
			return StartupSnippet{Notice: startupDestinationChangedNotice}
		},
	}
	alias := "production"
	spec, err := handlers.spec(api.OpenTerminalSessionRequest{Kind: api.OpenTerminalSessionRequestKindSsh, Alias: &alias}, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}

	got := spec.Startup()

	if got.Notice != startupDestinationChangedNotice || len(got.Commands) != 0 {
		t.Fatalf("startup = %#v, want only the notice", got)
	}
}
