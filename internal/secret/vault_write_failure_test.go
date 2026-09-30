package secret_test

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

var errInjectedVaultFault = errors.New("injected vault write fault")

// vaultWriteFaultFileSystem は、仕掛けたあとの vault のファイルへの書き込みを
// 1 回だけ失敗させる。rename そのものか、rename のあとのディレクトリの同期かを選ぶ。
type vaultWriteFaultFileSystem struct {
	storage.FileSystem
	vaultPath           string
	failRename          bool
	failSyncAfterRename bool
	// failRollback は、同期の失敗のあと、巻き戻しで vault を戻す rename も失敗させる。
	// プロセスが書き込みの途中で落ちたときと同じく、保留記録が残る。
	failRollback bool
	syncFailed   bool
	renamed      bool
}

func (f *vaultWriteFaultFileSystem) Rename(oldPath, newPath string) error {
	if newPath == f.vaultPath && f.failRename {
		f.failRename = false
		return errInjectedVaultFault
	}
	if newPath == f.vaultPath && f.failRollback && f.syncFailed {
		f.failRollback = false
		return errInjectedVaultFault
	}
	err := f.FileSystem.Rename(oldPath, newPath)
	if err == nil && newPath == f.vaultPath {
		f.renamed = true
	}
	return err
}

func (f *vaultWriteFaultFileSystem) SyncDir(path string) error {
	if f.renamed && f.failSyncAfterRename && path == filepath.Dir(f.vaultPath) {
		f.failSyncAfterRename = false
		f.syncFailed = true
		return errInjectedVaultFault
	}
	return f.FileSystem.SyncDir(path)
}

// vault だけを書く変更が途中で失敗しても、履歴の画面から「完了させる」ことの
// できる保留記録を残さない。残ると、完了させたあとディスクは新しい vault、
// メモリは古い vault になり、消したはずのシークレットを配り続ける。
func TestAFailedVaultWriteLeavesNoPendingRecordAndKeepsMemoryWithDisk(t *testing.T) {
	cases := map[string]func(*vaultWriteFaultFileSystem){
		"rename fails": func(fileSystem *vaultWriteFaultFileSystem) {
			fileSystem.failRename = true
		},
		"directory sync after the rename fails": func(fileSystem *vaultWriteFaultFileSystem) {
			fileSystem.renamed = false
			fileSystem.failSyncAfterRename = true
		},
	}
	for name, arm := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
				t.Fatal(err)
			}
			fileSystem := &vaultWriteFaultFileSystem{FileSystem: storage.OSFileSystem{}}
			workspace, err := storage.NewWorkspace(fileSystem, home)
			if err != nil {
				t.Fatal(err)
			}
			fileSystem.vaultPath = filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
			manager := storage.NewManager(workspace, time.Now, rand.Reader)
			service := secret.NewService(workspace, manager, time.Now)
			if err := service.Initialise(passphrase); err != nil {
				t.Fatal(err)
			}
			if err := service.SetCredential(secret.KindKeyPassphrase, "deploy", "old secret"); err != nil {
				t.Fatal(err)
			}

			arm(fileSystem)
			if err := service.DeleteCredential(secret.KindKeyPassphrase, "deploy"); !errors.Is(err, errInjectedVaultFault) {
				t.Fatalf("DeleteCredential = %v, want the injected fault", err)
			}

			pending, err := manager.Pending()
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 0 {
				t.Fatalf("the failed vault write left a pending record: %#v", pending)
			}
			if value, err := service.Credential(secret.KindKeyPassphrase, "deploy"); err != nil || value != "old secret" {
				t.Fatalf("Credential after the failed delete = %q, %v; want the old secret", value, err)
			}
			if err := service.SetCredential(secret.KindKeyPassphrase, "deploy", "new secret"); err != nil {
				t.Fatalf("the next vault write = %v", err)
			}
		})
	}
}

// 片付けた保留記録が vault のファイルに触れていれば、解錠中の vault を読み直す。
// 触れていなければ読み直さない。
func TestReloadAfterRecoveryReadsTheVaultOnlyWhenTheRecordTouchedIt(t *testing.T) {
	stale, home := newService(t)
	if err := stale.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	writer := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := writer.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := writer.SetCredential(secret.KindKeyPassphrase, "deploy", "written elsewhere"); err != nil {
		t.Fatal(err)
	}

	unrelated := filepath.Join(workspace.Root(), "config")
	if err := stale.ReloadAfterRecovery([]string{unrelated}); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.Credential(secret.KindKeyPassphrase, "deploy"); err == nil {
		t.Fatal("a record that did not touch the vault reloaded it")
	}

	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	if err := stale.ReloadAfterRecovery([]string{unrelated, vaultPath}); err != nil {
		t.Fatal(err)
	}
	if value, err := stale.Credential(secret.KindKeyPassphrase, "deploy"); err != nil || value != "written elsewhere" {
		t.Fatalf("Credential after the reload = %q, %v", value, err)
	}
}

// vaultOnlyWriters は、vault のファイルだけを書く書き手ごとの変更である。
var vaultOnlyWriters = map[string]func(*secret.Service) error{
	"set": func(service *secret.Service) error {
		return service.SetCredential(secret.KindKeyPassphrase, "deploy", "new secret")
	},
	"update": func(service *secret.Service) error {
		return service.UpdateCredential(secret.KindKeyPassphrase, "deploy", "release", "new secret")
	},
}

// newProductionWiredService は、本番の engine（app の buildServices）と同じく、世代
// バックアップを vault の鍵で封じ、保留記録を片付けたら vault を読み直す manager と
// service を組む。
func newProductionWiredService(t *testing.T, workspace *storage.Workspace) (*secret.Service, *storage.Manager) {
	t.Helper()
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	manager.Seal = service.SealBackup
	manager.Unseal = service.OpenBackup
	manager.AfterRecovery = func(paths []string) {
		if err := service.ReloadAfterRecovery(paths); err != nil {
			t.Errorf("ReloadAfterRecovery = %v", err)
		}
	}
	return service, manager
}

// leavePendingVaultWrite は、masterPassword（空ならパスワードなし）の vault に
// write を加え、vault のファイルを置き換えたあとで落ちたときの保留記録を残す。
// 巻き戻すには、vault の鍵で封じた控えを開く必要がある。
func leavePendingVaultWrite(t *testing.T, masterPassword string, write func(*secret.Service) error) *storage.Workspace {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &vaultWriteFaultFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	fileSystem.vaultPath = filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	service, manager := newProductionWiredService(t, workspace)
	if err := service.Initialise(masterPassword); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "deploy", "old secret"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignCredential(secret.KindKeyPassphrase, "keys/id_old", "deploy"); err != nil {
		t.Fatal(err)
	}

	fileSystem.renamed = false
	fileSystem.failSyncAfterRename = true
	fileSystem.failRollback = true
	if err := write(service); !errors.Is(err, errInjectedVaultFault) {
		t.Fatalf("write = %v, want the injected fault", err)
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 || !pending[0].Entries[0].Committed || !pending[0].Entries[0].HasBackup {
		t.Fatalf("Pending = %#v, %v; want the record a crash after the rename leaves", pending, err)
	}
	return workspace
}

// パスワードなしの vault なら、vault だけを書く変更の保留記録は、どの書き手のもの
// でも起動時に片付く。控えを開く鍵は、起動時の自動解錠で手に入る。
func TestStartupRecoversAPendingRecordFromEveryVaultOnlyWriter(t *testing.T) {
	for name, write := range vaultOnlyWriters {
		t.Run(name, func(t *testing.T) {
			workspace := leavePendingVaultWrite(t, "", write)

			restarted, restartedManager := newProductionWiredService(t, workspace)
			if err := restarted.AutoUnlock(); err != nil {
				t.Fatalf("AutoUnlock = %v", err)
			}
			if remaining, err := restartedManager.Pending(); err != nil || len(remaining) != 0 {
				t.Fatalf("Pending after start = %#v, %v; want the record recovered", remaining, err)
			}
			if value, err := restarted.Credential(secret.KindKeyPassphrase, "deploy"); err != nil || value != "old secret" {
				t.Fatalf("Credential after start = %q, %v; want the value before the interrupted write", value, err)
			}
		})
	}
}

// マスターパスワードの vault は起動時に開けない。控えを開く鍵が無いので、保留記録は
// 残して engine を起動し、利用者が解錠したあとで巻き戻せるようにする。
func TestStartupWithALockedVaultLeavesAVaultWriteThatNeedsTheKeyForAfterUnlock(t *testing.T) {
	for name, write := range vaultOnlyWriters {
		t.Run(name, func(t *testing.T) {
			workspace := leavePendingVaultWrite(t, passphrase, write)

			restarted, restartedManager := newProductionWiredService(t, workspace)
			if err := restarted.AutoUnlock(); err != nil {
				t.Fatalf("AutoUnlock = %v, want the engine to start with the record left pending", err)
			}
			remaining, err := restartedManager.Pending()
			if err != nil || len(remaining) != 1 || !remaining[0].CanRollback {
				t.Fatalf("Pending after start = %#v, %v; want the record left for after unlock", remaining, err)
			}

			if err := restarted.Unlock(passphrase); err != nil {
				t.Fatalf("Unlock = %v", err)
			}
			if err := restartedManager.Rollback(remaining[0].ID); err != nil {
				t.Fatalf("Rollback after unlock = %v", err)
			}
			if value, err := restarted.Credential(secret.KindKeyPassphrase, "deploy"); err != nil || value != "old secret" {
				t.Fatalf("Credential after the rollback = %q, %v; want the value before the interrupted write", value, err)
			}
		})
	}
}

// 資格情報だけを書く変更は、vault がまだ作られていないときも、ロック中と同じに断る。
// 接続の保存が ErrNoVault を返すのとは分けてある（TestConnectionSecretsMutationRequiresAnUnlockedExistingVault）。
func TestAVaultOnlyWriteReportsAMissingVaultAsLocked(t *testing.T) {
	service, _ := newService(t)
	writers := map[string]func() error{
		"set": func() error { return service.SetCredential(secret.KindPassword, "shared", "value") },
		"update": func() error {
			return service.UpdateCredential(secret.KindPassword, "shared", "renamed", "value")
		},
	}
	for name, write := range writers {
		if err := write(); !errors.Is(err, secret.ErrLocked) {
			t.Errorf("%s without a vault = %v, want ErrLocked", name, err)
		}
	}
}
