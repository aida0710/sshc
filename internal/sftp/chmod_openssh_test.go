package sftp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSSHBatchPermissionsChangeFilesAndDirectoriesWithoutChangingALinkTarget(t *testing.T) {
	client := openOpenSSHTestClient(t)
	directory := t.TempDir()
	group := filepath.Join(directory, "group")
	if err := os.Mkdir(group, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(group, "inside.txt")
	outside := filepath.Join(directory, "outside.txt")
	standalone := filepath.Join(directory, "standalone.txt")
	for _, filePath := range []string{inside, outside, standalone} {
		if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(group, "link")); err != nil {
		t.Fatal(err)
	}
	selection := []ChmodEntry{}
	for _, candidate := range []string{group, standalone} {
		remotePath := filepath.ToSlash(candidate)
		info, err := client.Lstat(remotePath)
		if err != nil {
			t.Fatal(err)
		}
		selection = append(selection, ChmodEntry{Path: remotePath, ExpectedRevision: metadataRevision(info)})
	}
	service := Service{Open: func(_ context.Context, _ string) (Remote, error) { return client, nil }}
	plan, err := service.PrepareChmod(t.Context(), ChmodRequest{Alias: "fixture", Entries: selection,
		Options: ChmodOptions{FileMode: 0o640, DirectoryMode: 0o750, Recursive: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer plan.Close()
	if plan.Files != 2 || plan.Directories != 1 || plan.SkippedSymlinks != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	changed, err := plan.Apply(t.Context(), plan.Revision)
	if err != nil || changed.Applied != 3 {
		t.Fatalf("permission changes = %+v, %v", changed, err)
	}
	for filePath, permissions := range map[string]os.FileMode{inside: 0o640, standalone: 0o640, group: 0o750, outside: 0o600} {
		info, err := os.Stat(filePath)
		if err != nil || info.Mode().Perm() != permissions {
			t.Fatalf("permissions for %s = %v; expected %o, error %v", filePath, info, permissions, err)
		}
	}
}
