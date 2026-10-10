//go:build linux || darwin

package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalTextSaveRemovesItsPrivateDirectoryAfterPublishing(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, filename, "after")
	children, err := os.ReadDir(directory)
	if err != nil || len(children) != 1 {
		t.Fatalf("staging left after save = %v, %v", children, err)
	}
}

func TestLocalTextPublicationAndCleanupRespectThePinnedStagingDirectory(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := openLocalTextStagingFile(parent, "stage")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.cleanup(parent)
	if _, err := staging.file.WriteString("mine"); err != nil {
		t.Fatal(err)
	}
	if err := staging.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := parent.Rename("stage", "moved"); err != nil {
		t.Fatal(err)
	}
	if err := parent.Mkdir("stage", localTextStagingDirectoryPermission); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "stage", localTextStagingContentsName)
	writeLocalMutationFile(t, replacement, "external")
	if err := publishLocalTextReplacement(parent, staging, "notes.txt"); !errors.Is(err, ErrConflict) {
		t.Fatalf("replaced staging directory publication = %v; want conflict", err)
	}
	assertLocalMutationFile(t, filename, "before")
	// Simulate replacement immediately after the identity check. The native
	// rename must still take our pinned contents, never the substituted entry.
	if err := renameLocalTextStagingContents(parent, staging, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, filename, "mine")
	staging.cleanup(parent)
	assertLocalMutationFile(t, replacement, "external")
	if _, err := os.Stat(filepath.Join(directory, "moved", localTextStagingContentsName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pinned staging contents survived cleanup: %v", err)
	}
}

func TestLocalTextPublicationRefusesAStagingDirectoryMadePublic(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := openLocalTextStagingFile(parent, "stage")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.cleanup(parent)
	if err := parent.Chmod("stage", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := publishLocalTextReplacement(parent, staging, "notes.txt"); !errors.Is(err, ErrConflict) {
		t.Fatalf("public staging directory publication = %v; want conflict", err)
	}
	assertLocalMutationFile(t, filename, "before")
}
