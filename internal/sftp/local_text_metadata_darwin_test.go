package sftp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLocalTextSaveRetainsDarwinACLAndExtendedAttributes(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	if err := unix.Setxattr(filename, "org.sshc.editor-test", []byte("retained"), 0); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", filename).CombinedOutput(); err != nil {
		t.Fatalf("ACL fixture: %s: %v", output, err)
	}
	before := captureDarwinTextMetadataFixture(t, filename)
	if len(before.platformMetadata) <= darwinAttributeLengthBytes {
		t.Fatal("ACL fixture has no native ACL")
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	after := captureDarwinTextMetadataFixture(t, filename)
	if before.revision != after.revision {
		t.Fatalf("access policy changed: before=%+v after=%+v", before, after)
	}
	assertLocalMutationFile(t, filename, "after")
}

func TestLocalTextSaveRefusesChangedDarwinACL(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", filename).CombinedOutput(); err != nil {
		t.Fatalf("ACL change: %s: %v", output, err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("ACL conflict = %v", err)
	}
	assertLocalMutationFile(t, filename, "before")
}

func captureDarwinTextMetadataFixture(t *testing.T, filename string) localTextMetadata {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	metadata, err := captureLocalTextMetadata(file)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestLocalTextStagingRemovesInheritedDarwinACLBeforeCreatingContents(t *testing.T) {
	directory := t.TempDir()
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read,file_inherit,directory_inherit", directory).CombinedOutput(); err != nil {
		t.Fatalf("directory ACL fixture: %s: %v", output, err)
	}
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staged, err := openLocalTextStagingFile(parent, "private.tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer staged.cleanup(parent)
	directoryFile, err := staged.directory.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer directoryFile.Close()
	if err := verifyLocalTextStagingDirectory(directoryFile); err != nil {
		t.Fatalf("staging directory is not private: %v", err)
	}
	metadata, err := captureLocalTextMetadata(staged.file)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.platformMetadata) != darwinAttributeLengthBytes || metadata.mode.Perm() != localTextStagingPermission {
		t.Fatalf("staging access policy = %+v", metadata)
	}
}
