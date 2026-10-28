package terminalopen

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/terminal"
	"sshc/internal/terminalrestore"
	"sshc/internal/terminalworkspace"
)

// connectedProcess は、Connect が返したものをそのまま見分けるための印である。メソッドは呼ばない。
type connectedProcess struct{ terminal.Process }

func sshRequest(alias string) Request {
	return Request{Kind: terminal.KindSSH, Alias: alias, Size: terminal.Size{Cols: 80, Rows: 24}}
}

func TestAShellOpensWithTheStoredDefaultUnlessTheRequestNamesAnother(t *testing.T) {
	opener := Opener{
		ShellProfiles: func() []platform.ShellProfile {
			return []platform.ShellProfile{
				{ID: "default", Label: "sh", Path: "/bin/sh", Argv0: "-sh"},
				{ID: "fish", Label: "fish", Path: "/usr/bin/fish", Argv0: "-fish"},
				{ID: "zsh", Label: "zsh", Path: "/bin/zsh", Argv0: "-zsh"},
			}
		},
		DefaultShellProfile: func() string { return "fish" },
	}
	stored, err := opener.shellSpec(Request{Kind: terminal.KindShell})
	if err != nil || stored.Command.Path != "/usr/bin/fish" || stored.Command.Argv0 != "-fish" {
		t.Fatalf("stored profile = %#v, %v", stored.Command, err)
	}
	oneShot := "zsh"
	overridden, err := opener.shellSpec(Request{Kind: terminal.KindShell, ShellProfile: &oneShot})
	if err != nil || overridden.Command.Path != "/bin/zsh" {
		t.Fatalf("one-shot profile = %#v, %v", overridden.Command, err)
	}
	opener.DefaultShellProfile = func() string { return "powershell" }
	fallback, err := opener.shellSpec(Request{Kind: terminal.KindShell})
	if err != nil || fallback.Command.Path != "/bin/sh" {
		t.Fatalf("cross-platform fallback = %#v, %v", fallback.Command, err)
	}
	unsafe := "/bin/sh -c id"
	if _, err := opener.shellSpec(Request{Kind: terminal.KindShell, ShellProfile: &unsafe}); !errors.Is(err, platform.ErrUnknownShellProfile) {
		t.Fatalf("unsafe profile = %v", err)
	}
}

func TestAnSSHSessionChangesToTheRequestedDirectoryAndThenRunsTheStartupSnippet(t *testing.T) {
	opener := Opener{
		Connect: func(context.Context, string, terminal.Size) (terminal.Process, error) { return connectedProcess{}, nil },
		Startup: func(alias string) StartupSnippet {
			if alias != "production" {
				t.Fatalf("startup alias = %q", alias)
			}
			return StartupSnippet{Command: "tmux attach"}
		},
	}
	cases := map[string][]string{
		"/srv/release 2": {`cd -- '/srv/release 2'`, "tmux attach"},
		// A directory some login shell would read as code is not typed; the
		// session starts in the home directory and the snippet still runs.
		"/srv/it's": {"tmux attach"},
	}
	for directory, want := range cases {
		request := sshRequest("production")
		request.WorkingDirectory = &directory
		spec, err := opener.spec(request)
		if err != nil {
			t.Fatal(err)
		}
		got := spec.Startup()
		if !slices.Equal(got.Commands, want) || got.Notice != "" {
			t.Fatalf("startup for %q = %#v, want the commands %q", directory, got, want)
		}
	}
}

// 送らなかったことの案内は、コマンドの代わりに接続のたびの知らせとして渡す。Ready を
// 待ってからターミナルへ書くのは terminal.Session である（sendStartup）。
func TestAHeldBackStartupSnippetIsHandedOverAsANoticeWithoutACommand(t *testing.T) {
	opener := Opener{
		Connect: func(context.Context, string, terminal.Size) (terminal.Process, error) { return connectedProcess{}, nil },
		Startup: func(string) StartupSnippet { return StartupSnippet{Notice: "held back"} },
	}
	spec, err := opener.spec(sshRequest("production"))
	if err != nil {
		t.Fatal(err)
	}
	if got := spec.Startup(); got.Notice != "held back" || len(got.Commands) != 0 {
		t.Fatalf("startup = %#v, want only the notice", got)
	}
}

func TestAnSSHSessionOutlivesTheRequestThatOpenedItAndGetsTheConnectedProcessItself(t *testing.T) {
	underlying := connectedProcess{}
	var connectContext context.Context
	opener := Opener{
		Connect: func(ctx context.Context, _ string, _ terminal.Size) (terminal.Process, error) {
			connectContext = ctx
			return underlying, nil
		},
	}
	spec, err := opener.spec(sshRequest("production"))
	if err != nil {
		t.Fatal(err)
	}
	request, cancelRequest := context.WithCancel(context.Background())
	process, err := spec.Open(request, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	// 包みを返すと、包みが素通ししない能力（接続の進捗など）が黙って消える。
	if process != terminal.Process(underlying) {
		t.Fatalf("spec.Open returned %T, want the connected process itself", process)
	}
	cancelRequest()
	if err := connectContext.Err(); err != nil {
		t.Fatalf("the SSH session context ended with the HTTP request: %v", err)
	}
}

func TestAnSSHSessionNeedsAValidAlias(t *testing.T) {
	opener := Opener{Connect: func(context.Context, string, terminal.Size) (terminal.Process, error) { return connectedProcess{}, nil }}
	if _, err := opener.spec(sshRequest("")); !errors.Is(err, ErrMissingAlias) {
		t.Fatalf("no alias = %v, want ErrMissingAlias", err)
	}
	if _, err := opener.spec(sshRequest("-oProxyCommand=id")); err == nil {
		t.Fatal("an alias that reads as an option was accepted")
	}
}

func TestReopeningLetsGoOfAWorkspaceADirectoryAndAShellProfileThatAreGone(t *testing.T) {
	start := t.TempDir()
	opener := Opener{
		ShellProfiles: func() []platform.ShellProfile {
			return []platform.ShellProfile{{ID: "default", Label: "sh", Path: "/bin/sh", Argv0: "-sh"}}
		},
		StartDirectory:   func() string { return start },
		RequireWorkspace: func(string) error { return errors.New("removed") },
	}
	saved := terminalrestore.Session{
		Kind: terminal.KindShell, ShellProfile: "fish", Workspace: "gone",
		WorkingDirectory: filepath.Join(start, "removed"),
	}
	spec, err := opener.reopeningSpec(saved)
	if err != nil {
		t.Fatalf("reopeningSpec = %v, want a spec from what is still usable", err)
	}
	if spec.Workspace != terminalworkspace.DefaultID || spec.ShellProfile != "default" || spec.Command.Dir != start {
		t.Fatalf("reopened = workspace %q, profile %q, directory %q", spec.Workspace, spec.ShellProfile, spec.Command.Dir)
	}
}

func TestReopeningKeepsAWorkspaceThatStillExists(t *testing.T) {
	opener := Opener{
		Connect:          func(context.Context, string, terminal.Size) (terminal.Process, error) { return connectedProcess{}, nil },
		RequireWorkspace: func(string) error { return nil },
	}
	spec, err := opener.reopeningSpec(terminalrestore.Session{Kind: terminal.KindSSH, Alias: "production", Workspace: "ops"})
	if err != nil || spec.Workspace != "ops" {
		t.Fatalf("reopened = %q, %v; want the saved workspace", spec.Workspace, err)
	}
}
