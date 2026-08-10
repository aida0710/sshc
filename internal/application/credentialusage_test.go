package application

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeCredentialUsageConfig(t *testing.T, workspaceRoot, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspaceRoot, "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestKeyHostsFindsDirectAndInheritedIdentityFiles(t *testing.T) {
	service, workspace := newTestService(t)
	writeCredentialUsageConfig(t, workspace.Root(), ""+
		"Host direct\n"+
		"\tIdentityFile ~/.ssh/keys/id_team\n"+
		"Host inherited-a inherited-b\n"+
		"\tHostName example.test\n"+
		"Host *\n"+
		"\tIdentityFile ~/.ssh/keys/id_global\n")

	got, err := service.KeyHosts([]string{"keys/id_team", "keys/id_global", "keys/unused"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["keys/id_team"], []string{"direct"}) {
		t.Fatalf("team hosts = %#v, want direct", got["keys/id_team"])
	}
	if !slices.Equal(got["keys/id_global"], []string{"direct", "inherited-a"}) {
		t.Fatalf("global hosts = %#v, want direct and inherited-a", got["keys/id_global"])
	}
	if got["keys/unused"] == nil || len(got["keys/unused"]) != 0 {
		t.Fatalf("unused hosts = %#v, want non-nil empty", got["keys/unused"])
	}
}

func TestKeyHostsSortsDeduplicatesAndDoesNotGuessRelativePaths(t *testing.T) {
	service, workspace := newTestService(t)
	writeCredentialUsageConfig(t, workspace.Root(), ""+
		"Host zed\n"+
		"\tIdentityFile ~/.ssh/id_shared\n"+
		"\tIdentityFile ~/.ssh/id_shared\n"+
		"Host alpha\n"+
		"\tIdentityFile ~/.ssh/id_shared\n"+
		"Host zed\n"+
		"\tIdentityFile ~/.ssh/id_other\n"+
		"Host relative\n"+
		"\tIdentityFile id_relative\n")

	got, err := service.KeyHosts([]string{"id_shared", "id_other", "id_relative"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["id_shared"], []string{"alpha", "zed"}) {
		t.Fatalf("shared hosts = %#v, want alpha and zed once", got["id_shared"])
	}
	if !slices.Equal(got["id_other"], []string{"zed"}) {
		t.Fatalf("other hosts = %#v, want cumulative zed", got["id_other"])
	}
	if got["id_relative"] == nil || len(got["id_relative"]) != 0 {
		t.Fatalf("relative hosts = %#v, want non-nil empty", got["id_relative"])
	}
}
