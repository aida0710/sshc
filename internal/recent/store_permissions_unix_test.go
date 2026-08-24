//go:build !windows

package recent_test

import (
	"os"
	"testing"
)

func assertStoredDocumentPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode = %o, want no group or other access", info.Mode().Perm())
	}
}
