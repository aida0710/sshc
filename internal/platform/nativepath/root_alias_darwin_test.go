//go:build darwin

package nativepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRootAliasCanonicalizesMacOSSystemVarAlias(t *testing.T) {
	info, err := os.Lstat("/var")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Skip("this macOS installation does not expose /var as a symlink")
	}
	path := filepath.Join(string(filepath.Separator), "var", "folders", "workspace")
	got, err := ResolveRootAlias(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/private/var/folders/workspace"; got != want {
		t.Fatalf("ResolveRootAlias(%q) = %q, want %q", path, got, want)
	}
}

func TestResolveRootAliasLeavesLowerComponentsUnresolved(t *testing.T) {
	path := filepath.Join(string(filepath.Separator), "Users", "person", "linked", "workspace")
	got, err := ResolveRootAlias(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("ResolveRootAlias(%q) = %q, want unchanged path", path, got)
	}
}
