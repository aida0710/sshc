package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListReadsConcreteAliasesFromConfigAndIncludes(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(ssh, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(ssh, filepath.FromSlash(name)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config", "Host zeta second\n  HostName zeta.example\nInclude conf.d/*.conf\nHost *\n  User ops\n")
	write("conf.d/10-work.conf", "Host alpha !blocked web-* single-?\n  HostName alpha.example\nHost second\n")

	var stdout, stderr bytes.Buffer
	if code := runList(home, &stdout, &stderr); code != 0 {
		t.Fatalf("runList = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "alpha\nsecond\nzeta\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestListTreatsAMissingConfigAsAnEmptyList(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runList(t.TempDir(), &stdout, &stderr); code != 0 {
		t.Fatalf("runList = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

// OpenSSH は shell メタ文字や先頭の '-' を含む Host alias も読む。この一覧は補完の
// 候補になるため、validate.Alias が拒む alias を stdout へ出してはならない。
func TestListSkipsAliasesThisApplicationRefusesToEvaluate(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host $(id)\n  HostName evil.example\nHost alpha\n  HostName alpha.example\n" +
		"Host -oProxyCommand=id\nHost \x1b[2Jboom\n"
	if err := os.WriteFile(filepath.Join(ssh, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runList(home, &stdout, &stderr); code != 0 {
		t.Fatalf("runList = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "alpha\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	for _, refused := range []string{"$(id)", "-oProxyCommand=id"} {
		if !strings.Contains(stderr.String(), refused) {
			t.Errorf("stderr = %q, want it to report %q", stderr.String(), refused)
		}
	}
	// 落とした alias を報告するときも、端末制御文字はそのまま書かない。
	if strings.ContainsRune(stderr.String(), '\x1b') {
		t.Errorf("stderr = %q, want the escape sequence quoted", stderr.String())
	}
}
