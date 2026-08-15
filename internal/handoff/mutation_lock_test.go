//go:build unix

package handoff

import (
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

	acquired := make(chan func(), 1)
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
