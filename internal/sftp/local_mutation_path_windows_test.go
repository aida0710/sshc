package sftp

import (
	"errors"
	"testing"
)

func TestWindowsLocalMutationsRejectDriveRelativeDeviceAndUNCTraversalPaths(t *testing.T) {
	for _, value := range []string{`C:folder`, `\folder`, `\\?\C:\folder`, `\\.\C:\folder`, `//server/share/../outside`, `C:/folder/stream:secret`} {
		if _, err := cleanLocalMutationPath(value); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("path %q = %v", value, err)
		}
	}
	for _, value := range []string{"C:/", "//server/share", "//server/share/"} {
		if _, err := cleanLocalMutationPath(value); !errors.Is(err, ErrRootOperation) {
			t.Errorf("root %q = %v", value, err)
		}
	}
	for _, name := range []string{"CON", "NUL.txt", "nul.TXT", "CON .txt", "COM1", "LPT2.log", "stream:secret", "folder.", "folder ", ".SSHC-DOWNLOAD-ACTIVE"} {
		if validLocalMutationName(name) {
			t.Errorf("accepted Windows name %q", name)
		}
	}
	for _, name := range []string{"notes.txt", ".profile", "console.txt", "COM10.txt"} {
		if !validLocalMutationName(name) {
			t.Errorf("rejected ordinary Windows name %q", name)
		}
	}
}

func TestWindowsLocalMutationPathsRetainDriveAndUNCRoots(t *testing.T) {
	for _, directory := range []string{`C:\folder\`, `\\server\share\folder\`} {
		joined := joinLocalMutationPath(directory, "child")
		cleaned, err := cleanLocalMutationPath(joined)
		if err != nil || cleaned != joined {
			t.Errorf("joined %q = %q, %v", joined, cleaned, err)
		}
	}
	if joined := joinLocalMutationPath("//server/share/", "child"); joined != "//server/share/child" {
		t.Errorf("UNC root lost: %q", joined)
	}
}
