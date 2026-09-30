package secret_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

func TestABackupWrittenWhileTheNewKeyIsDerivedIsReSealedWithIt(t *testing.T) {
	service, workspace, manager := recoveryService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(workspace.Root(), "known_hosts")
	if err := os.WriteFile(knownHosts, []byte("bastion ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 通常の commit が workspace のロックを持ち、バックアップを封じる直前まで進んだことを知らせる。
	sealing := make(chan struct{})
	var signalSealing sync.Once
	manager.Seal = func(plaintext []byte) ([]byte, error) {
		signalSealing.Do(func() { close(sealing) })
		return service.SealBackup(plaintext)
	}
	committed := make(chan error, 1)
	derivations := 0
	commitReachedSealing := false
	envelope.OnDerive = func(step func()) {
		derivations++
		// 1 回目は今のパスワードの確認、2 回目が新しい鍵の導出である。
		if derivations == 2 {
			go func() {
				_, err := manager.Commit(storage.Request{
					Operation: "known_hosts.add",
					Changes: []storage.Change{{
						Path: knownHosts, Contents: []byte("bastion ssh-ed25519 AAAA\nweb ssh-ed25519 BBBB\n"),
						Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte("bastion ssh-ed25519 AAAA\n"))},
					}},
				})
				committed <- err
			}()
			// Commit がバックアップを封じる前に終わったら（失敗した、封じるバックアップが
			// なかった）、ここで待ち続けずに先へ進め、あとで前提が崩れたとして落とす。
			select {
			case <-sealing:
				commitReachedSealing = true
			case err := <-committed:
				committed <- err
			}
		}
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	const next = "a different master password"
	if err := service.ChangeMasterPassword(context.Background(), passphrase, next); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	envelope.OnDerive = nil
	if derivations < 2 {
		t.Fatalf("ChangeMasterPassword derived a key %d times; the concurrent commit never started", derivations)
	}
	if !commitReachedSealing {
		t.Fatalf("the concurrent commit finished before sealing a backup: %v", <-committed)
	}
	if err := <-committed; err != nil {
		t.Fatalf("concurrent known_hosts commit = %v", err)
	}

	backups := filepath.Join(workspace.StateDir(), storage.BackupDirectoryName)
	opened := 0
	if err := filepath.WalkDir(backups, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		sealed, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := service.OpenBackup(sealed); err != nil {
			t.Errorf("%s does not open with the new master password: %v", path, err)
		}
		opened++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if opened == 0 {
		t.Fatal("the concurrent commit left no backup to check")
	}
	if err := service.ChangeMasterPassword(context.Background(), next, "yet another master password"); err != nil {
		t.Fatalf("the next ChangeMasterPassword = %v", err)
	}
}
