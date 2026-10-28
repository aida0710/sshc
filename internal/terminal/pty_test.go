//go:build unix

package terminal_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestARealPseudoTerminalCarriesTheOutputAndTheExitStatus(t *testing.T) {
	shell := lookProgram(t, "sh")

	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	t.Cleanup(func() {
		registry.ForceClose()
		_ = registry.Wait()
	})
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "sh",
		Size: terminal.Size{Cols: 80, Rows: 24},
		// 子を生かしたまま出力を読む。macOS の PTY は、短命な子が reader の開始前に
		// slave を閉じると、既に書いた出力を返さず EOF になる場合がある。
		Command: terminal.Command{
			Path:      shell,
			Arguments: []string{"-c", "printf '%s\\n' embedded-terminal-canary; read ignored"},
		},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(string(snapshotOf(session)), "embedded-terminal-canary") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := string(snapshotOf(session)); !strings.Contains(got, "embedded-terminal-canary") {
		t.Fatalf("the scrollback never received the output: %q", got)
	}
	if _, err := session.Write([]byte("\n")); err != nil {
		t.Fatalf("release the shell: %v", err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && session.Exit() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	info := session.Exit()
	if info == nil {
		t.Fatal("the session never recorded an exit")
	}
	if info.Code != 0 || info.Signal != "" {
		t.Fatalf("exit = %+v, want a clean one", info)
	}
	if info.At.IsZero() {
		t.Fatal("the exit carried no time")
	}
}

func TestTheShellReadsLinesAsUTF8(t *testing.T) {
	shell := lookProgram(t, "sh")
	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	t.Cleanup(func() {
		registry.ForceClose()
		_ = registry.Wait()
	})
	// 行単位の入力で日本語の Backspace が1文字を消すには、起動した時点で IUTF8 が要る。
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "sh",
		Size: terminal.Size{Cols: 200, Rows: 24},
		Command: terminal.Command{
			Path:      shell,
			Arguments: []string{"-c", "printf 'modes:%s:end\\n' \"$(stty -a | tr '\\n' ' ')\"; read ignored"},
		},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(string(snapshotOf(session)), ":end") {
		time.Sleep(5 * time.Millisecond)
	}
	got := string(snapshotOf(session))
	if !strings.Contains(got, ":end") {
		t.Fatalf("stty never answered: %q", got)
	}
	if !strings.Contains(got, " iutf8") || strings.Contains(got, "-iutf8") {
		t.Fatalf("the terminal settings lack iutf8: %q", got)
	}
}

func TestOpeningAMissingProgramFails(t *testing.T) {
	registry := &terminal.Registry{Start: terminal.NewStarter(), Limits: terminal.DefaultLimits}
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind:    terminal.KindShell,
		Command: terminal.Command{Path: "/nonexistent/embedded-terminal"},
	}); err == nil {
		t.Fatal("Open() accepted a program that does not exist")
	}
	if len(registry.Sessions()) != 0 {
		t.Fatal("a failed start left a session behind")
	}
}

func lookProgram(t *testing.T, name string) string {
	t.Helper()
	for _, directory := range []string{"/bin", "/usr/bin"} {
		path := directory + "/" + name
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	t.Skipf("%s was not found; this machine cannot run the real pseudo-terminal check", name)
	return ""
}
