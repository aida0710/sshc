package terminal

import "testing"

func TestRemoteDirectoriesAreChangedIntoWithTheTildeOutsideTheQuotes(t *testing.T) {
	cases := map[string]string{
		"~":                 "cd",
		"~/src/app":         `cd -- ~/'src/app'`,
		"/srv/project dir":  `cd -- '/srv/project dir'`,
		"/srv/$HOME (copy)": `cd -- '/srv/$HOME (copy)'`,
	}
	for directory, want := range cases {
		accepted, err := CheckRemoteWorkingDirectory(directory)
		if err != nil {
			t.Fatalf("CheckRemoteWorkingDirectory(%q) = %v", directory, err)
		}
		if got, ok := ChangeRemoteDirectoryCommand(accepted); !ok || got != want {
			t.Errorf("ChangeRemoteDirectoryCommand(%q) = %q, %v; want %q", directory, got, ok, want)
		}
	}
}

func TestADirectoryThatSomeLoginShellWouldReadAsCodeIsNotTyped(t *testing.T) {
	for _, directory := range []string{
		"/srv/project's files", // sh, bash, zsh: ends the quotes
		`/srv/dir\`,            // fish: escapes the closing quote
		"/srv/a’; calc",        // PowerShell: a typographic quote ends the quotes
		"/srv/a & calc",        // cmd.exe: runs the next command
		"/srv/%PATH%",          // cmd.exe: expands
		"/srv/a`b",             // PowerShell: escapes
	} {
		accepted, err := CheckRemoteWorkingDirectory(directory)
		if err != nil {
			t.Fatalf("CheckRemoteWorkingDirectory(%q) = %v, want it accepted and only the cd skipped", directory, err)
		}
		if command, ok := ChangeRemoteDirectoryCommand(accepted); ok {
			t.Errorf("ChangeRemoteDirectoryCommand(%q) = %q, want no command", directory, command)
		}
	}
}

func TestRemoteWorkingDirectoriesOutsideTheAcceptedShapesAreRefused(t *testing.T) {
	for _, candidate := range []string{"", "relative", "/srv/../etc", "/srv\nwhoami", "/srv\x7f", "/srv/\u009b31m", "/srv/\u202etxt.exe", "~root", "~/", "~/../etc", "~/..", "~/a/../b", "~//etc", "//host/share"} {
		if _, err := CheckRemoteWorkingDirectory(candidate); err == nil {
			t.Errorf("accepted %q", candidate)
		}
	}
}
