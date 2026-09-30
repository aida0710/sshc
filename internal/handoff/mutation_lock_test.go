package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Write と Remove が別々の lock を取ると、先に secret を比べた Remove が後発の
// Write を消せる。二つの actor が同じ lock で直列になることをここで表明する。
func TestMutationLockSerializesActors(t *testing.T) {
	directory := t.TempDir()
	firstRelease, err := lockMutation(directory)
	if err != nil {
		t.Fatalf("first lockMutation = %v", err)
	}
	firstReleased := false
	defer func() {
		if !firstReleased {
			firstRelease()
		}
	}()

	acquired := make(chan func() error, 1)
	errors := make(chan error, 1)
	go func() {
		release, err := lockMutation(directory)
		if err != nil {
			errors <- err
			return
		}
		acquired <- release
	}()

	select {
	case release := <-acquired:
		release()
		t.Fatal("second actor acquired the mutation lock before the first released it")
	case err := <-errors:
		t.Fatalf("second lockMutation = %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	firstRelease()
	firstReleased = true
	select {
	case release := <-acquired:
		release()
	case err := <-errors:
		t.Fatalf("second lockMutation = %v", err)
	case <-time.After(time.Second):
		t.Fatal("second actor did not acquire the mutation lock after release")
	}
}

// 一度も書かれていない state directory からの Remove は、何もしない。ロックのために
// ディレクトリを作らない。
func TestRemoveFromAMissingStateDirectoryCreatesNothing(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "sshc")
	if err := Remove(directory, "any secret"); err != nil {
		t.Fatalf("Remove from a missing directory = %v", err)
	}
	if _, err := os.Lstat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Remove created the state directory: %v", err)
	}
}
