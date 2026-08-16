//go:build unix

package acceptance_test

import (
	"os"
	"testing"
)

// handoff が私的であることは、Unix では mode で言える。
func assertHandoffIsPrivate(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat handoff = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff mode = %04o, want 0600", info.Mode().Perm())
	}
}
