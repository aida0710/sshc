package terminal

import "testing"

func TestWindowsConsoleDefaultTitleCountsAsNoTitle(t *testing.T) {
	const windowsPowerShell = `C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe`
	for name, title := range map[string]string{
		"not elevated":                windowsPowerShell,
		"elevated on Japanese":        `管理者: ` + windowsPowerShell,
		"elevated on English":         `Administrator: ` + windowsPowerShell,
		"elevated on French":          `Administrateur : ` + windowsPowerShell,
		"path with spaces":            `C:\Program Files\PowerShell\7\pwsh.exe`,
		"another drive and case":      `d:\tools\APP.EXE`,
		"a path longer than MaxTitle": `C:\Program Files\A Very Long Directory Name\Another Very Long Directory Name\program.exe`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := withoutExecutablePath(title); got != "" {
				t.Fatalf("withoutExecutablePath(%q) = %q", title, got)
			}
		})
	}
}

func TestCommandProcessorTitleKeepsTheRunningCommand(t *testing.T) {
	for title, want := range map[string]string{
		`C:\WINDOWS\system32\cmd.exe - ping localhost`:              "ping localhost",
		`管理者: C:\WINDOWS\system32\cmd.exe - ping localhost`:         "ping localhost",
		`C:\WINDOWS\system32\cmd.exe - C:\tools\build.exe --fast`:   `C:\tools\build.exe --fast`,
		`Administrator: C:\WINDOWS\system32\cmd.exe - npm  install`: "npm  install",
	} {
		if got := withoutExecutablePath(title); got != want {
			t.Errorf("withoutExecutablePath(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestTitlesAProgramChoseAreKept(t *testing.T) {
	for _, title := range []string{
		"",
		"vim notes.md",
		"powershell.exe",
		"aida@host: ~/src",
		"Windows PowerShell",
		"管理者: Windows PowerShell",
		`C:\Users\someone\src`,
		`C:\Users\someone\notes.txt`,
		"/usr/bin/bash",
		`build: C:\tools\build.exe failed`,
	} {
		if got := withoutExecutablePath(title); got != title {
			t.Errorf("withoutExecutablePath(%q) = %q", title, got)
		}
	}
}

func TestOSCObserverReportsNoTitleForAnExecutablePathOfAnyLength(t *testing.T) {
	observer, seen := newTestObserver()
	// With the elevation prefix, this is longer than MaxTitle and would be cut in the middle of the path.
	observer.Observe([]byte(`` + "\x1b]0;" + `Administrator: C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe` + "\a"))
	if len(seen.titles) != 1 || seen.titles[0] != "" {
		t.Fatalf("titles = %q", seen.titles)
	}
}

func TestSessionShowsItsFallbackInsteadOfAnExecutablePath(t *testing.T) {
	observer, _ := newTestObserver()
	session := newTitleTestSession("edge")
	observer.reports.Title = func(title string) { session.acceptTitle(1, title) }

	observer.Observe([]byte("\x1b]0;vim notes.md\a"))
	if view := session.View(); view.Title != "vim notes.md" {
		t.Fatalf("program title view = %+v", view.Presentation)
	}
	observer.Observe([]byte("\x1b]0;" + `管理者: C:\WINDOWS\System32\WindowsPowerShell\v1.0\powershell.exe` + "\a"))
	if view := session.View(); view.Title != "edge" || view.Presentation.TitleSource != TitleConnection {
		t.Fatalf("executable path view = %+v", view.Presentation)
	}
}
