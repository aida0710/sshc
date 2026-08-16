//go:build windows

package acceptance_test

import (
	"testing"

	"sshc/internal/platform/windowsacl"
)

// Windows の mode は書き込みビットしか運ばないので、`0600` はそこでは何も
// 言っていない。**私的であることを決めるのは DACL である。** handoff を書くのは
// windowsacl であり、ここで確かめるのはその同じ契約そのものである。
func assertHandoffIsPrivate(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatalf("IsRestrictedToCurrentUser(%q) = %v", path, err)
	}
	if !restricted {
		t.Fatalf("%q is not restricted to the current user", path)
	}
}
