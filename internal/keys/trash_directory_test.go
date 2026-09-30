package keys

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func trashEntryDirectory(root, entryID string) string {
	return filepath.Join(root, StateDirectoryName, "trash", entryID)
}

// 復元と完全削除は、空になったごみ箱のエントリのディレクトリも同じトランザクションで
// 取り除く。
func TestRestoreAndPurgeLeaveNoEmptyTrashEntryDirectory(t *testing.T) {
	service, root := newTrashService(t)

	trashed, err := service.Trash(ItemID("id_work"))
	if err != nil {
		t.Fatalf("Trash error = %v", err)
	}
	if _, err := service.Restore(trashed.EntryID); err != nil {
		t.Fatalf("Restore error = %v", err)
	}
	if _, err := os.Lstat(trashEntryDirectory(root, trashed.EntryID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the entry directory survived the restore: %v", err)
	}

	trashed, err = service.Trash(ItemID("id_work"))
	if err != nil {
		t.Fatalf("Trash error = %v", err)
	}
	if _, err := service.Purge(trashed.EntryID); err != nil {
		t.Fatalf("Purge error = %v", err)
	}
	if _, err := os.Lstat(trashEntryDirectory(root, trashed.EntryID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the entry directory survived the purge: %v", err)
	}
}

// manifest に無いファイルがエントリにあっても、完全削除は止まらない。そのファイルと
// ディレクトリは残す。
func TestAFileTheManifestDoesNotNameDoesNotBlockThePurge(t *testing.T) {
	service, root := newTrashService(t)
	trashed, err := service.Trash(ItemID("id_work"))
	if err != nil {
		t.Fatalf("Trash error = %v", err)
	}
	stray := filepath.Join(trashEntryDirectory(root, trashed.EntryID), "notes.txt")
	if err := os.WriteFile(stray, []byte("left by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	purged, err := service.Purge(trashed.EntryID)
	if err != nil {
		t.Fatalf("Purge error = %v", err)
	}
	if len(purged.Removed) != 2 {
		t.Fatalf("removed = %#v, want the key pair", purged.Removed)
	}
	if _, err := os.Lstat(stray); err != nil {
		t.Fatalf("the file the manifest does not name was touched: %v", err)
	}
}

// ごみ箱へ移せないと分かった削除は、エントリのディレクトリを作らない。
func TestATrashThatIsRefusedCreatesNoEntryDirectory(t *testing.T) {
	service, root := newTrashService(t)
	// 同じ鍵の公開鍵が、秘密鍵と同じ名前で別のディレクトリにもある。ごみ箱の
	// エントリは平らなので、二つを同じ名前で置けない。
	public, err := os.ReadFile(filepath.Join(root, "id_work.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "copies"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "copies", "id_work"), public, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Trash(ItemID("id_work")); !errors.Is(err, ErrTrashNameConflict) {
		t.Fatalf("Trash error = %v, want ErrTrashNameConflict", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, StateDirectoryName, "trash"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("trash = %d entries, want none from a refused trash", len(entries))
	}
}
