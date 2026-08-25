package storage

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const mutationLockHelperEnvironment = "SSHC_STORAGE_MUTATION_LOCK_HELPER"

func TestWorkspaceMutationLockCrossProcess(t *testing.T) {
	if os.Getenv(mutationLockHelperEnvironment) != "" {
		runMutationLockHelper()
		return
	}

	workspace := newTestWorkspace(t)
	release, err := workspace.lockMutation()
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			release()
		}
	})

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestWorkspaceMutationLockCrossProcess$")
	command.Env = append(os.Environ(),
		mutationLockHelperEnvironment+"=1",
		"SSHC_STORAGE_MUTATION_LOCK_HOME="+workspace.Home(),
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = command.Stdout
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "attempting" {
		t.Fatalf("helper readiness = %q, %v", line, err)
	}

	acquired := make(chan string, 1)
	go func() {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			acquired <- "read error: " + readErr.Error()
			return
		}
		acquired <- strings.TrimSpace(line)
	}()
	select {
	case line := <-acquired:
		t.Fatalf("helper crossed the process lock early: %s", line)
	case <-time.After(150 * time.Millisecond):
	}

	release()
	released = true
	select {
	case line := <-acquired:
		if line != "acquired" {
			t.Fatalf("helper result = %q", line)
		}
	case <-ctx.Done():
		t.Fatal("helper did not acquire the released process lock")
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMutationLockRejectsAReplacedRootSymlink(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := NewWorkspace(OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}

	original := root + "-original"
	if err := os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}

	previous := workspaceMutationLockDirectory
	workspaceMutationLockDirectory = func(workspace *Workspace) (string, error) {
		return workspace.StateDir(), nil
	}
	t.Cleanup(func() { workspaceMutationLockDirectory = previous })

	release, err := workspace.lockMutation()
	if release != nil {
		release()
		t.Fatal("a refused workspace lock returned a release function")
	}
	if !errors.Is(err, ErrSymlinkPath) {
		t.Fatalf("lockMutation after root replacement = %v, want ErrSymlinkPath", err)
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "sshc")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("lockMutation created state outside the workspace: %v", statErr)
	}
}

func runMutationLockHelper() {
	workspace, err := NewWorkspace(OSFileSystem{}, os.Getenv("SSHC_STORAGE_MUTATION_LOCK_HOME"))
	if err != nil {
		fmt.Println("workspace error:", err)
		os.Exit(2)
	}
	fmt.Println("attempting")
	release, err := workspace.lockMutation()
	if err != nil {
		fmt.Println("lock error:", err)
		os.Exit(3)
	}
	fmt.Println("acquired")
	release()
}
