//go:build windows

package snippets

import (
	"testing"

	"sshc/internal/platform/windowsacl"
)

func assertSnippetDocumentPrivate(t *testing.T, path string) {
	t.Helper()
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		t.Fatal(err)
	}
	if !restricted {
		t.Fatalf("%q does not have the required private Windows DACL", path)
	}
}
