package secret_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"sshc/internal/storage"
)

func TestLargeBackgroundBackupSurvivesMasterPasswordChange(t *testing.T) {
	service, workspace, manager := recoveryService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace.StateDir(), "backgrounds", "large.png")
	if err := workspace.EnsureDirectory(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	previous := bytes.Repeat([]byte("original"), 256<<10)
	if _, err := manager.Commit(storage.Request{Operation: "background.create", Changes: []storage.Change{{Path: path, Contents: previous}}}); err != nil {
		t.Fatal(err)
	}
	committed, err := manager.Commit(storage.Request{Operation: "background.replace", Changes: []storage.Change{{
		Path: path, Contents: []byte("replacement"),
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(previous)},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	const nextPassphrase = "new background backup master password"
	if err := service.ChangeMasterPassword(passphrase, nextPassphrase); err != nil {
		t.Fatalf("rekey large backup: %v", err)
	}
	service.Lock()
	if err := service.Unlock(nextPassphrase); err != nil {
		t.Fatal(err)
	}
	restored, err := manager.ReadBackup(filepath.Join(committed.BackupDir, "sshc", "backgrounds", "large.png"))
	if err != nil || !bytes.Equal(restored, previous) {
		t.Fatalf("read rekeyed backup: %v", err)
	}
	if _, err := manager.Commit(storage.Request{Operation: "background.restore", Changes: []storage.Change{{
		Path: path, Contents: restored,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte("replacement"))},
	}}}); err != nil {
		t.Fatalf("restore large asset: %v", err)
	}
	restored, err = workspace.ReadTransactionFile(path)
	if err != nil || !bytes.Equal(restored, previous) {
		t.Fatalf("restored asset: %v", err)
	}
}
