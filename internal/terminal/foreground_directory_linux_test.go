package terminal_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestForegroundDirectoryIsWhereTheRunningProgramWorks(t *testing.T) {
	shell := lookProgram(t, "sh")
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := &terminal.Registry{
		Start:  terminal.NewStarter(),
		Limits: func() terminal.Limits { return terminal.Limits{MaxSessions: 2, Scrollback: 16 << 10} },
	}
	t.Cleanup(func() {
		registry.ForceClose()
		_ = registry.Wait()
	})
	// The shell starts elsewhere and moves, as `cd project && claude` does.
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "sh",
		Size: terminal.Size{Cols: 80, Rows: 24},
		Command: terminal.Command{
			Path: shell, Dir: "/",
			Arguments: []string{"-c", "cd \"$1\" && read ignored", "sh", directory},
		},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}

	got := ""
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got, err = session.ForegroundDirectory(); err == nil && got == directory {
			return
		}
	}
	t.Fatalf("ForegroundDirectory() = %q, %v; want %q", got, err, directory)
}
