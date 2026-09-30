package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// linkUnsupported stands in for FAT, exFAT and SMB mounts, which cannot make
// a hard link. Linux answers EPERM for them.
func linkUnsupported(*os.Root, string, string) error {
	return &os.LinkError{Op: "link", Err: syscall.EPERM}
}

func openPublishRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := os.WriteFile(filepath.Join(directory, ".sshc-download-part"), []byte("downloaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, directory
}

func TestPublishWithoutReplaceFinishesOnAFilesystemWithoutHardLinks(t *testing.T) {
	root, directory := openPublishRoot(t)

	if err := localLink(linkUnsupported).publishWithoutReplace(root, ".sshc-download-part", "report.txt"); err != nil {
		t.Fatalf("publish = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(directory, "report.txt"))
	if err != nil || string(contents) != "downloaded" {
		t.Fatalf("published file = %q, %v", contents, err)
	}
}

func TestPublishWithoutReplaceKeepsAnExistingFileWhenHardLinksAreUnsupported(t *testing.T) {
	root, directory := openPublishRoot(t)
	existing := filepath.Join(directory, "report.txt")
	if err := os.WriteFile(existing, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := localLink(linkUnsupported).publishWithoutReplace(root, ".sshc-download-part", "report.txt")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("publish over an existing file = %v, want ErrAlreadyExists", err)
	}
	if contents, _ := os.ReadFile(existing); string(contents) != "mine" {
		t.Fatalf("existing file became %q", contents)
	}
}

func TestPublishWithoutReplaceReportsAnExistingFileFromTheHardLink(t *testing.T) {
	root, directory := openPublishRoot(t)
	if err := os.WriteFile(filepath.Join(directory, "report.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := publishLocalWithoutReplace(root, ".sshc-download-part", "report.txt"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("publish over an existing file = %v, want ErrAlreadyExists", err)
	}
}
