package storage

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestManagersSharingWorkspaceSerializePreconditionAndCommit(t *testing.T) {
	managerOne, workspace := newTestManager(t)
	original := []byte("Host old\n")
	target := writeWorkspaceFile(t, workspace, "config", string(original), FilePermission)

	managerTwo := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x6e}, 4096)))
	firstValidated := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondValidated := make(chan struct{})
	managerOne.Validate = func(Request) error {
		close(firstValidated)
		<-releaseFirst
		return nil
	}
	managerTwo.Validate = func(Request) error {
		close(secondValidated)
		return nil
	}

	precondition := Precondition{Exists: true, Digest: Digest(original)}
	firstResult := make(chan error, 1)
	go func() {
		_, err := managerOne.Commit(Request{
			Operation: "test.first",
			Changes:   []Change{{Path: target, Contents: []byte("Host first\n"), Precondition: precondition}},
		})
		firstResult <- err
	}()
	<-firstValidated

	secondResult := make(chan error, 1)
	go func() {
		_, err := managerTwo.Commit(Request{
			Operation: "test.second",
			Changes:   []Change{{Path: target, Contents: []byte("Host second\n"), Precondition: precondition}},
		})
		secondResult <- err
	}()

	select {
	case <-secondValidated:
		t.Fatal("a second manager passed its precondition while the first mutation was in progress")
	case err := <-secondResult:
		t.Fatalf("a second manager returned before the first mutation completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-firstResult; err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if err := <-secondResult; err == nil {
		t.Fatal("second commit unexpectedly overwrote the first commit")
	} else {
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("second commit error = %v, want ConflictError", err)
		}
	}

	contents, err := workspace.FileSystem().ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "Host first\n" {
		t.Fatalf("target = %q, want first commit", contents)
	}
}
