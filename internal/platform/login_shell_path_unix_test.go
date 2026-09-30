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

	"github.com/creack/pty"
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

// loginShellTerminalHelperEnvironment が 1 のとき、補助プロセスとして WithLoginShellPath を呼ぶ。
const loginShellTerminalHelperEnvironment = "SSHC_TEST_LOGIN_SHELL_TERMINAL_HELPER"

// loginShellTerminalHelperTimeout は、補助プロセスが PATH を待つ上限である。本番の上限
// （loginShellPathTimeout）より短くし、端末で止まったシェルを上限の切れで見分ける。
const loginShellTerminalHelperTimeout = 2 * time.Second

// 補助プロセスの終了コード。0 は PATH を受け取れたことを表す。
const (
	loginShellTerminalHelperFailed    = 2
	loginShellTerminalHelperWrongPath = 3
)

func TestWithLoginShellPathDoesNotStopWhenTheEngineHasAControllingTerminal(t *testing.T) {
	// 対話シェルが job control を始めるときと同じく、制御端末を読みに行く。engine の
	// 端末を継いだ背景の process group なら、ここで SIGTTIN を受けて止まる。
	environment := proxyTestShell(t, `
(read -r _ < /dev/tty) 2>/dev/null
export PATH="$SSHC_TEST_PATH"
exec /bin/sh -c "$3"
`)
	path := t.TempDir() + "/bin:/usr/bin:/bin"
	// 前面や tmux の engine と同じく、制御端末を持つ session leader として補助プロセスを動かす。
	command := exec.Command(os.Args[0], "-test.run=^TestWithLoginShellPathTerminalHelperProcess$")
	command.Env = append(environment, "SSHC_TEST_PATH="+path, loginShellTerminalHelperEnvironment+"=1")
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = terminal.Close()
	}()

	err = command.Wait()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == loginShellTerminalHelperFailed {
		t.Fatal("the login shell stopped on the engine's controlling terminal until the deadline")
	}
	if err != nil {
		t.Fatalf("helper process = %v", err)
	}
}

func TestWithLoginShellPathTerminalHelperProcess(t *testing.T) {
	if os.Getenv(loginShellTerminalHelperEnvironment) != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginShellTerminalHelperTimeout)
	defer cancel()
	updated, err := WithLoginShellPath(ctx, os.Environ())
	if err != nil {
		os.Exit(loginShellTerminalHelperFailed)
	}
	if !slices.Contains(updated, "PATH="+os.Getenv("SSHC_TEST_PATH")) {
		os.Exit(loginShellTerminalHelperWrongPath)
	}
	os.Exit(0)
}
