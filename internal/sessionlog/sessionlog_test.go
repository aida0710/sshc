package sessionlog

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestEverySessionKeepsALogUntilARuleLeavesItOut(t *testing.T) {
	rules, err := CompileRules(`
# Production hosts print secrets in their banners.
prod-*
!prod-web
@local
`)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{
		"bastion": true, "prod-db": false, "PROD-DB": false, "prod-web": true, LocalShellName: false,
	} {
		if got := rules.Keeps(name); got != want {
			t.Errorf("Keeps(%q) = %v, want %v", name, got, want)
		}
	}
	if empty, _ := CompileRules(""); !empty.Keeps("anything") {
		t.Fatal("with no rule, a session keeps no log")
	}
}

func TestAPatternThatIsNotOneIsRefused(t *testing.T) {
	for _, text := range []string{"[web", "!", strings.Repeat("x", MaxRulesBytes+1), "web\x00"} {
		if _, err := CompileRules(text); err == nil {
			t.Errorf("CompileRules(%q) succeeded", text)
		}
	}
	rules, err := CompileRules(`\#hash` + "\n" + `\!bang`)
	if err != nil || rules.Keeps("#hash") || rules.Keeps("!bang") {
		t.Fatalf("an escaped # or ! is a pattern: %v", err)
	}
}

func TestTheDefaultFolderIsWhereTheSystemKeepsLogsOutsideSSH(t *testing.T) {
	none := func(string) string { return "" }
	for goos, want := range map[string]string{
		"linux":   "/home/aida/.local/state/sshc/session-logs",
		"freebsd": "/home/aida/.local/state/sshc/session-logs",
		"darwin":  "/home/aida/Library/Logs/sshc/sessions",
	} {
		if got, err := DefaultDirectory(goos, "/home/aida", none); err != nil || got != filepath.FromSlash(want) {
			t.Errorf("DefaultDirectory(%s) = %q, %v; want %q", goos, got, err, want)
		}
	}
	// XDG_STATE_HOME counts only when it is absolute, which on this system is a path of its own form.
	stateHome := t.TempDir()
	state := func(name string) string {
		if name == "XDG_STATE_HOME" {
			return stateHome
		}
		return ""
	}
	if got, _ := DefaultDirectory("linux", "/home/aida", state); got != filepath.Join(stateHome, "sshc", "session-logs") {
		t.Errorf("with XDG_STATE_HOME, DefaultDirectory = %q", got)
	}
	if _, err := Directory("", "android", "/data/user/0/app", none); err != ErrUnsupported {
		t.Errorf("Directory on Android = %v, want ErrUnsupported", err)
	}
	if got, err := Directory("~/logs", "linux", "/home/aida", none); err != nil || got != filepath.FromSlash("/home/aida/logs") {
		t.Errorf("Directory(~/logs) = %q, %v", got, err)
	}
	if _, err := Directory("logs", "linux", "/home/aida", none); err == nil {
		t.Error("a relative folder was taken")
	}
}

func TestALogFileNameNamesTheSessionAndWhenItStartedWithNothingASystemRefuses(t *testing.T) {
	started := time.Date(2026, 11, 8, 10, 15, 2, 0, time.Local)
	if got := FileName("web-01", "0123456789abcdef", started); got != "web-01-20261108-101502-01234567.log" {
		t.Errorf("FileName = %q", got)
	}
	if got := FileName("ops@db:2222", "abc", started); got != "ops@db_2222-20261108-101502-abc.log" {
		t.Errorf("FileName for a destination = %q", got)
	}
}

func readLog(t *testing.T, write func(*Log)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "web.log")
	log, err := Open(path, terminal.Size{Cols: 40, Rows: 3})
	if err != nil {
		t.Fatal(err)
	}
	write(log)
	log.Close()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestALogHoldsTheLinesTheTerminalShowedAndWhatIsStillOnTheScreen(t *testing.T) {
	got := readLog(t, func(log *Log) {
		log.Note("web に接続しました")
		log.Write([]byte("\x1b[32maida@web\x1b[0m:~$ ls\r\na.txt\r\nb.txt\r\n"))
		log.Write([]byte("aida@web:~$ "))
	})
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "[sshc] ") || !strings.HasSuffix(lines[0], " web に接続しました") {
		t.Fatalf("log = %q", got)
	}
	if want := []string{"aida@web:~$ ls", "a.txt", "b.txt", "aida@web:~$"}; strings.Join(lines[1:], "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", lines[1:], want)
	}
}

func TestASecretTheShellEchoesIsHiddenInTheLog(t *testing.T) {
	got := readLog(t, func(log *Log) {
		log.Hide([]string{"s3cret-token"})
		log.Write([]byte("$ deploy --token=s3cret-token\r\n"))
	})
	if strings.Contains(got, "s3cret-token") || !strings.Contains(got, "deploy --token=[secret]") {
		t.Fatalf("log = %q", got)
	}
}

func TestTheLogAndItsFolderAreTheOwnersOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps them private with an ACL (internal/privatefile)")
	}
	root := t.TempDir()
	path := filepath.Join(root, "logs", "web.log")
	log, err := Open(path, terminal.Size{Cols: 40, Rows: 3})
	if err != nil {
		t.Fatal(err)
	}
	log.Close()
	for target, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v; want %v", target, info.Mode().Perm(), err, want)
		}
	}
}

func TestANoteComesAfterTheLinesShownBeforeIt(t *testing.T) {
	got := readLog(t, func(log *Log) {
		log.Write([]byte("one\r\ntwo\r\n$ "))
		log.Note("接続が切れました。")
		log.Write([]byte("\r\nthree\r\nfour\r\n"))
		log.Note("セッションが終わりました。")
	})
	var order []string
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if strings.HasPrefix(line, "[sshc] ") {
			line = line[strings.LastIndex(line, " ")+1:]
		}
		order = append(order, line)
	}
	want := []string{"one", "two", "$", "接続が切れました。", "three", "four", "セッションが終わりました。"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("log = %q, want the lines in %q's order", got, want)
	}
}
