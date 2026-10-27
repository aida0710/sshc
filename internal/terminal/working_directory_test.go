package terminal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOSC7DirectoryIsTheDecodedPathOfAFileURL(t *testing.T) {
	cases := map[string]string{
		"file://edge/home/aida/src":    "/home/aida/src",
		"file:///home/aida/my%20notes": "/home/aida/my notes",
		"file://edge/":                 "/",
		"http://edge/home/aida":        "",
		"file://user@edge/home":        "",
		"file://edge/home?query":       "",
		"file://edge/home#fragment":    "",
		"file://edge/home/a%0Ab":       "",
		"file://edge/" + strings.Repeat("a", maxWorkingDirectoryBytes): "",
		"not a url %": "",
	}
	for payload, want := range cases {
		got, ok := parseOSC7Directory(payload)
		if got != want || ok != (want != "") {
			t.Errorf("parseOSC7Directory(%q) = %q, %v; want %q", payload, got, ok, want)
		}
	}
}

func TestBashDefaultTitleGivesTheDirectory(t *testing.T) {
	cases := map[string]string{
		"aida@edge: ~/src/sshc":   "~/src/sshc",
		"aida@edge: ~":            "~",
		"root@edge: /var/log":     "/var/log",
		"aida@edge: vim notes.md": "",
		"vim ~/notes.md":          "",
		"edge: ~/src":             "",
		"a b@edge: ~/src":         "",
		"@edge: ~/src":            "",
		"aida@: ~/src":            "",
		"aida@edge:~/src":         "",
		"aida@edge: ~other/src":   "",
		"aida@edge: /" + strings.Repeat("a", MaxTitle): "",
	}
	for title, want := range cases {
		got, ok := workingDirectoryFromTitle(title)
		if got != want || ok != (want != "") {
			t.Errorf("workingDirectoryFromTitle(%q) = %q, %v; want %q", title, got, ok, want)
		}
	}
}

func TestOSCObserverReportsTheOSC7Directory(t *testing.T) {
	var directories []string
	observer := newOSCObserver(oscReports{Directory: func(directory string) { directories = append(directories, directory) }})
	observer.Observe([]byte("\x1b]7;file://edge/home/aida/src\a$ \x1b]7;ftp://edge/x\x1b\\"))
	if len(directories) != 1 || directories[0] != "/home/aida/src" {
		t.Fatalf("directories = %q", directories)
	}
}

func TestSessionKeepsTheLatestReportedDirectoryAcrossTitlesAndOSC7(t *testing.T) {
	session := newTitleTestSession("edge")
	if got := session.WorkingDirectory(); got != "" {
		t.Fatalf("a new session knows a directory: %q", got)
	}
	session.acceptTitle(1, "aida@edge: ~/src")
	session.acceptTitle(1, "vim notes.md")
	if got := session.WorkingDirectory(); got != "~/src" {
		t.Fatalf("after a program title the directory is %q, want ~/src", got)
	}
	session.acceptWorkingDirectory(1, "/srv/app")
	session.acceptWorkingDirectory(0, "/stale")
	if got := session.WorkingDirectory(); got != "/srv/app" {
		t.Fatalf("directory = %q, want /srv/app", got)
	}
}

func TestALocalShellOffersOnlyDirectoriesThisMachineCanStartIn(t *testing.T) {
	session := newTitleTestSession("")
	session.kind = KindShell
	session.acceptTitle(1, "aida@edge: ~/src")
	if got := session.WorkingDirectory(); got != "" {
		t.Fatalf("a local shell offers the title's %q", got)
	}
	absolute := filepath.ToSlash(t.TempDir())
	if runtime.GOOS == "windows" {
		absolute = "/" + absolute
	}
	session.acceptWorkingDirectory(1, absolute)
	if got := session.WorkingDirectory(); got != strings.TrimPrefix(absolute, "/") && got != absolute {
		t.Fatalf("WorkingDirectory() = %q for the report %q", got, absolute)
	}
}
