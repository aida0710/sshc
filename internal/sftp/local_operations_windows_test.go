//go:build windows

package sftp_test

import (
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/sftp"
)

func TestLocalListingOnWindowsOpensTheHomeTypedWithABackslash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	documents := filepath.Join(home, "Documents")
	if err := os.Mkdir(documents, 0o700); err != nil {
		t.Fatal(err)
	}
	for typed, want := range map[string]string{`~\`: home, `~\Documents`: documents} {
		listing, err := sftp.ListLocal(typed)
		if err != nil {
			t.Fatalf("ListLocal(%q) = %v", typed, err)
		}
		if listing.Path != filepath.ToSlash(want) {
			t.Fatalf("ListLocal(%q).Path = %q, want %q", typed, listing.Path, filepath.ToSlash(want))
		}
	}
}
