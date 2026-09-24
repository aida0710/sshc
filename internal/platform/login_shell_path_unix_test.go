//go:build !windows && !android && !ios

package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func proxyTestShell(t *testing.T, script string) []string {
	t.Helper()
	home := t.TempDir()
	shell := filepath.Join(home, "shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return []string{"HOME=" + home, "SHELL=" + shell, "PATH=/usr/bin:/bin"}
}

func TestWithLoginShellPathUsesShellPathWithoutChangingOtherVariables(t *testing.T) {
	environment := proxyTestShell(t, `
test "$1" = -i || exit 1
test "$2" = -c || exit 1
printf 'startup message\n'
printf 'startup secret\n' >&2
export PATH="$SSHC_TEST_PATH"
export SSHC_TEST_SECRET=changed
exec /bin/sh -c "$3"
`)
	// 引用符や改行を含むdirectoryも、シェルのコードとして展開してはならない。
	path := t.TempDir() + `/bin ' " $(false)` + "\nnext:/usr/bin:/bin"
	environment = append(environment, "SSHC_TEST_PATH="+path, "SSHC_TEST_SECRET=original")
	before := slices.Clone(environment)
	updated, err := WithLoginShellPath(context.Background(), environment)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(updated, "PATH="+path) || slices.Contains(updated, "PATH=/usr/bin:/bin") {
		t.Fatalf("PATH was not replaced: %q", updated)
	}
	if !slices.Contains(updated, "SSHC_TEST_SECRET=original") || !slices.Equal(before, environment) {
		t.Fatal("changed the engine environment or imported non-PATH variables")
	}
}

func TestWithLoginShellPathReadsZshLoginAndInteractiveStartup(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	home := t.TempDir()
	for name, content := range map[string]string{
		".zprofile": "export PATH=\"$HOME/login/bin:$PATH\"\nprint 'login banner'\n",
		".zshrc":    "export PATH=\"$HOME/interactive/bin:$PATH\"\nprint 'interactive banner'\n",
	} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	updated, err := WithLoginShellPath(context.Background(), []string{
		"HOME=" + home, "ZDOTDIR=" + home, "SHELL=" + zsh, "PATH=/usr/bin:/bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, entry := range updated {
		if value, found := strings.CutPrefix(entry, "PATH="); found {
			path = value
		}
	}
	want := filepath.Join(home, "interactive/bin") + ":" + filepath.Join(home, "login/bin") + ":"
	if !strings.HasPrefix(path, want) {
		t.Fatalf("PATH = %q, want prefix %q", path, want)
	}
}

func TestWithLoginShellPathKeepsInheritedPathWhenShellCannotReturnOne(t *testing.T) {
	for name, script := range map[string]string{
		"failed startup":    "printf 'private startup output'; exit 1\n",
		"missing marker":    "printf 'private startup output'\n",
		"empty path":        "printf '\\000sshc-path\\000\\000'\n",
		"unterminated path": "printf '\\000sshc-path\\000/some/bin'\n",
		"too much output":   "head -c 65537 /dev/zero\n",
	} {
		t.Run(name, func(t *testing.T) {
			environment := proxyTestShell(t, script)
			updated, err := WithLoginShellPath(context.Background(), environment)
			if err == nil || !slices.Equal(updated, environment) {
				t.Fatalf("environment changed or failure was hidden: %q, %v", updated, err)
			}
			if strings.Contains(err.Error(), "private startup output") {
				t.Fatal("shell startup output leaked into diagnostics")
			}
		})
	}
}

func TestWithLoginShellPathCancelsShellStartupAndItsChild(t *testing.T) {
	environment := proxyTestShell(t, "sleep 60\n")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	updated, err := WithLoginShellPath(ctx, environment)
	if !errors.Is(err, context.DeadlineExceeded) || !slices.Equal(updated, environment) {
		t.Fatalf("cancel = %q, %v", updated, err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("shell startup cancellation took %s", elapsed)
	}
}
