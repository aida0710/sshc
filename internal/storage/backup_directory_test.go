package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// 控えを 1 つも書かないトランザクションは、backups/<id> を作らない。SkipBackup の
// 置き換え、新しいファイルだけ、移動だけのどれでも、空のディレクトリを残さない。
func TestATransactionThatWritesNoBackupLeavesNoBackupDirectory(t *testing.T) {
	manager, workspace := newTestManager(t)
	config := writeWorkspaceFile(t, workspace, "config", "Host old\n", 0o644)
	added := filepath.Join(workspace.Root(), "added.conf")
	moved := filepath.Join(workspace.Root(), "moved.conf")

	for _, request := range []Request{
		{Operation: "sync.state", Changes: []Change{{
			Path: config, Contents: []byte("Host new\n"), SkipBackup: true,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host old\n"))},
		}}},
		{Operation: "config.add", Changes: []Change{{Path: added, Contents: []byte("Host added\n")}}},
		{Operation: "config.move", Moves: []Move{{
			From: added, To: moved,
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("Host added\n"))},
		}}},
	} {
		if _, err := manager.Commit(request); err != nil {
			t.Fatalf("%s: %v", request.Operation, err)
		}
	}

	entries, err := os.ReadDir(filepath.Join(workspace.StateDir(), backupDirectoryName))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("backups = %d entries, want none for transactions that wrote no backup", len(entries))
	}
	history, err := manager.History()
	if err != nil || len(history) != 3 {
		t.Fatalf("History = %d records, %v; want all three transactions recorded", len(history), err)
	}
}

func TestParentDirectoryCreatesNamesEachParentOnceAndNeverTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".ssh")
	got := ParentDirectoryCreates(root, []string{
		filepath.Join(root, "config"),
		filepath.Join(root, "keys", "work", "id_work"),
		filepath.Join(root, "keys", "work", "id_work.pub"),
		filepath.Join(root, "connections", "work.conf"),
	})
	want := []DirectoryCreate{
		{Path: filepath.Join(root, "keys", "work")},
		{Path: filepath.Join(root, "connections")},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ParentDirectoryCreates = %v, want %v", got, want)
	}
}
