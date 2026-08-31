package storage

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithSnapshotExcludesWorkspaceCommits(t *testing.T) {
	workspace := newTestWorkspace(t)
	manager := NewManager(workspace, time.Now, rand.Reader)
	target := filepath.Join(workspace.Root(), "config")
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	finished := make(chan error, 1)
	if err := manager.WithSnapshot(func() error {
		go func() {
			close(started)
			_, err := manager.Commit(Request{Operation: "snapshot-exclusion", Changes: []Change{{Path: target, Contents: []byte("new")}}})
			finished <- err
		}()
		<-started
		select {
		case err := <-finished:
			return fmt.Errorf("commit crossed snapshot barrier: %w", err)
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("commit did not resume after snapshot barrier")
	}
}

func TestPendingTransactionBlocksMutationsAndSnapshotsUntilRecovery(t *testing.T) {
	manager, workspace, first, _ := interruptedCommit(t)
	created := filepath.Join(workspace.Root(), "blocked.conf")
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending = %#v, %v", pending, err)
	}

	snapshotCalled := false
	attempts := []struct {
		name string
		run  func() error
	}{
		{name: "commit", run: func() error {
			_, err := manager.Commit(Request{Operation: "blocked.commit", Changes: []Change{{Path: created, Contents: []byte("blocked")}}})
			return err
		}},
		{name: "atomic commit", run: func() error {
			_, err := manager.CommitAtomic(Request{Operation: "blocked.atomic", Changes: []Change{{Path: created, Contents: []byte("blocked")}}})
			return err
		}},
		{name: "note", run: func() error {
			_, err := manager.Note("blocked.note", []string{first})
			return err
		}},
		{name: "snapshot", run: func() error {
			return manager.WithSnapshot(func() error {
				snapshotCalled = true
				return nil
			})
		}},
	}
	for _, attempt := range attempts {
		t.Run(attempt.name, func(t *testing.T) {
			err := attempt.run()
			if !errors.Is(err, ErrPendingTransaction) || !errors.Is(err, ErrWorkspaceBusy) {
				t.Fatalf("error = %v, want pending transaction/workspace busy", err)
			}
		})
	}
	if snapshotCalled {
		t.Fatal("snapshot observed a workspace with a pending transaction")
	}
	if _, statErr := os.Stat(created); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("blocked mutation created a file: %v", statErr)
	}
	stillPending, err := manager.Pending()
	if err != nil || len(stillPending) != 1 || stillPending[0].ID != pending[0].ID {
		t.Fatalf("Pending after refusals = %#v, %v", stillPending, err)
	}

	if err := manager.Complete(pending[0].ID); err != nil {
		t.Fatalf("Complete = %v", err)
	}
	if _, err := manager.Commit(Request{
		Operation: "after.recovery", Changes: []Change{{Path: created, Contents: []byte("allowed")}},
	}); err != nil {
		t.Fatalf("Commit after recovery = %v", err)
	}
}

func TestDiscardBackupPublishRunsBeforeMutationBarrierRelease(t *testing.T) {
	workspace := newTestWorkspace(t)
	manager := NewManager(workspace, time.Now, rand.Reader)
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace.Root(), "vault")
	entered := make(chan struct{})
	finished := make(chan error, 1)
	crossed := false
	_, err := manager.CommitAtomicDiscardBackupsAndPublish(Request{
		Operation: "publish-generation",
		Changes:   []Change{{Path: target, Contents: []byte("new-generation")}},
	}, func() {
		go func() {
			finished <- manager.WithSnapshot(func() error {
				close(entered)
				return nil
			})
		}()
		select {
		case <-entered:
			crossed = true
		case <-time.After(100 * time.Millisecond):
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if crossed {
		t.Fatal("a new workspace generation started before in-memory publication finished")
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waiting workspace generation did not resume")
	}
}

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
