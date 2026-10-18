//go:build windows

package enginelog

import (
	"path/filepath"
	"testing"

	"sshc/internal/platform/windowsacl"
)

func TestLogAndDirectoryHavePrivateACLs(t *testing.T) {
	stateDir := t.TempDir()
	file, err := Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	for _, path := range []string{Path(stateDir), filepath.Dir(Path(stateDir))} {
		private, err := windowsacl.IsRestrictedToCurrentUser(path)
		if err != nil || !private {
			t.Fatalf("ACL for %s: private = %v, err = %v", path, private, err)
		}
	}
}
