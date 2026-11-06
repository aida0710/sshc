package terminal_test

import (
	"context"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestTitleNamesTheCommandAnInteractiveShellRuns(t *testing.T) {
	shell := lookProgram(t, "sh")
	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	t.Cleanup(func() {
		registry.ForceClose()
		_ = registry.Wait()
	})
	// An interactive shell controls jobs: it puts each command in its own process
	// group and gives it the terminal, which is what the engine reads.
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "sh",
		Size:    terminal.Size{Cols: 80, Rows: 24},
		Command: terminal.Command{Path: shell, Dir: "/", Arguments: []string{"-i"}},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	waitForTitle := func(want string) {
		t.Helper()
		got := ""
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			if got = session.View().Presentation.DisplayTitle; got == want {
				return
			}
		}
		t.Fatalf("title = %q, want %q", got, want)
	}

	waitForTitle("sh")
	if _, err := session.Write([]byte("sleep 30\n")); err != nil {
		t.Fatal(err)
	}
	waitForTitle("sleep")
	if _, err := session.Write([]byte{0x03}); err != nil {
		t.Fatal(err)
	}
	waitForTitle("sh")
}
