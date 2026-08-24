//go:build windows

package recent_test

import (
	"testing"

	"sshc/internal/platform/windowsacl"
)

func assertStoredDocumentPrivate(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatal(err)
	}
	if !restricted {
		t.Fatalf("%q does not have the required private Windows DACL", path)
	}
}
