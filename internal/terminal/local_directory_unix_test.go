//go:build unix

package terminal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

func TestARealLocalShellStartsInTheSelectedFolderWithoutExecutingItsName(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "work $(touch injected) & 'notes'")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := terminal.ResolveLocalWorkingDirectory(filepath.ToSlash(directory))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	registry := &terminal.Registry{Start: terminal.NewStarter(), Limits: terminal.DefaultLimits}
	t.Cleanup(func() {
		registry.ForceClose()
		_ = registry.Wait()
	})
	session, err := registry.Open(t.Context(), terminal.Spec{
		Kind: terminal.KindShell, Title: "sh",
		Command: terminal.Command{
			Path: lookProgram(t, "sh"), Dir: resolved,
			// A non-login shell and an isolated HOME avoid reading user startup files.
			Env:       []string{"HOME=" + base, "PATH=/usr/bin:/bin", "TERM=xterm-256color"},
			Arguments: []string{"-c", "printf 'START=%s\\n' \"$PWD\"; read ignored"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, stream, ok := session.AttachFrom(0)
	if !ok {
		t.Fatal("the shell output could not be attached")
	}
	defer session.Detach(stream)
	wait, cancel := context.WithTimeout(t.Context(), testwait.Limit)
	defer cancel()
	output := string(replay.Data)
	for !strings.Contains(output, "START="+physical) {
		select {
		case chunk, open := <-stream.Output():
			if !open {
				t.Fatalf("shell exited before reporting its directory: %q", output)
			}
			output += string(chunk)
		case <-wait.Done():
			t.Fatalf("shell did not start in the selected folder: %q", output)
		}
	}
	for _, parent := range []string{base, directory} {
		if _, err := os.Stat(filepath.Join(parent, "injected")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the folder name was executed: %v", err)
		}
	}
}
