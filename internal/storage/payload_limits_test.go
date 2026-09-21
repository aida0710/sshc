package storage

import (
	"bytes"
	"errors"
	"testing"
)

func TestOversizedConfigIsRejectedBeforeChangingTheWorkspace(t *testing.T) {
	manager, workspace := newTestManager(t)
	path := writeWorkspaceFile(t, workspace, "config", "Host original\n", FilePermission)
	previous, err := workspace.FileSystem().ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Commit(Request{Operation: "config.edit", Changes: []Change{{
		Path: path, Contents: bytes.Repeat([]byte("x"), MaxFileSize+1),
		Precondition: Precondition{Exists: true, Digest: Digest(previous)},
	}}})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("oversized config write: %v", err)
	}
	current, err := workspace.FileSystem().ReadFile(path)
	if err != nil || !bytes.Equal(current, previous) {
		t.Fatalf("failed write changed the config: %v", err)
	}
}
