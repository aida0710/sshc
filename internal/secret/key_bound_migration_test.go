package secret_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/secret"
	"sshc/internal/snippets"
	"sshc/internal/storage"
)

// sshc/snippets.json が平文だった v0.6.0〜v0.20.x の sshc は、平文のスニペットと、
// pull がそれを置き換えたときの、外側の封の中に平文の JSON がある（入れ子に
// なっていない）控えを残した。どちらもロックを解除したときに今の形へ移し、中身を
// 失わず、マスターパスワードの変更も止めない。

const (
	// plaintextSnippets は、平文のまま残った sshc/snippets.json の中身。
	plaintextSnippets = `{"schemaVersion":1,"snippets":[{"id":"left-by-an-older-release"}]}`
	// unnestedSnippetsBackup は、入れ子でない控えの外側の封の中にある平文。
	unnestedSnippetsBackup = `{"schemaVersion":1,"snippets":[{"id":"replaced-by-a-pull"}]}`
	// operationMigrateKeyBoundArtifacts は、移行が履歴に残す Operation。
	operationMigrateKeyBoundArtifacts = "secret.migrate-key-bound-artifacts"
)

// legacySnippets は、旧バージョンが残したファイルの置き場所。
type legacySnippets struct {
	document string
	backup   string
}

func snippetsPath(workspace *storage.Workspace) string {
	return filepath.Join(workspace.Root(), filepath.FromSlash(snippets.PathRelative))
}

// registerSnippets は、本番の配線と同じく sshc/snippets.json を保護文書にする。
func registerSnippets(t *testing.T, service *secret.Service, workspace *storage.Workspace) {
	t.Helper()
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: snippetsPath(workspace),
		Validate: func(document []byte) error {
			if !json.Valid(document) {
				return errors.New("the snippets document is not JSON")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
}

// reopenWithSnippets は、engine の再起動と同じく、ロックしたサービスを組み直す。
func reopenWithSnippets(t *testing.T, home string) *secret.Service {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := newSealedService(workspace, time.Now)
	registerSnippets(t, service, workspace)
	return service
}

// placeLegacySnippets は、平文の sshc/snippets.json と、outer で外側だけを封じた
// 入れ子でない控えを置く。控えの記録は保持の期間に収まるよう、今の時刻で名付ける。
func placeLegacySnippets(t *testing.T, workspace *storage.Workspace, outer envelope.Key) legacySnippets {
	t.Helper()
	legacy := legacySnippets{
		document: snippetsPath(workspace),
		backup: filepath.Join(
			workspace.StateDir(), storage.BackupDirectoryName, time.Now().UTC().Format("20060102T150405.000")+"-deadbeef",
			filepath.FromSlash(snippets.PathRelative),
		),
	}
	unnested, err := outer.Seal([]byte(unnestedSnippetsBackup))
	if err != nil {
		t.Fatal(err)
	}
	// 古いバージョンの sshc も、どちらも非公開状態として書いた。Windows の読み口は
	// ACL を先に確かめるので、素の os.WriteFile で置くと移行まで届かない。
	acltest.WritePrivateFile(t, legacy.document, []byte(plaintextSnippets))
	acltest.WritePrivateFile(t, legacy.backup, unnested)
	return legacy
}

// vaultKey は、passphrase で開ける vault の鍵を返す。
func vaultKey(t *testing.T, home, passphrase string) envelope.Key {
	t.Helper()
	sealed, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	_, key, err := envelope.Open(sealed, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(key.Destroy)
	return key
}

// backupOuterKey は、入れ子でない控えの外側を封じた鍵を返す。
type backupOuterKey func(t *testing.T, home string) envelope.Key

// theVaultKey は、今の Vault の鍵を返す。
func theVaultKey(t *testing.T, home string) envelope.Key {
	t.Helper()
	return vaultKey(t, home, passphrase)
}

// anEarlierKeyGeneration は、同じマスターパスワードから作った、今の Vault とは別の世代の
// 鍵を返す。v0.16〜v0.20 の未対応の Vault の復旧とリセットは Vault だけを置き換え、控えを
// 封じ直さなかったので、この鍵で外側を封じた控えが残りうる。
func anEarlierKeyGeneration(t *testing.T, _ string) envelope.Key {
	t.Helper()
	key, err := envelope.Derive(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(key.Destroy)
	return key
}

// backupOuterKeys は、入れ子でない控えの外側を封じうる鍵の一覧。
var backupOuterKeys = map[string]backupOuterKey{
	"outer sealed by the vault key":             theVaultKey,
	"outer sealed by an earlier key generation": anEarlierKeyGeneration,
}

// assertCurrentForm は、スニペットが封じてあり、控えが入れ子で、どちらも元の中身を
// ロックを解除した service の鍵で開けることを確かめる。
func assertCurrentForm(t *testing.T, service *secret.Service, legacy legacySnippets) {
	t.Helper()
	document, err := os.ReadFile(legacy.document)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenDocument(document)
	if err != nil || !bytes.Equal(opened, []byte(plaintextSnippets)) {
		t.Fatalf("snippets = %q, %v; want the sealed %q", opened, err, plaintextSnippets)
	}
	backup, err := os.ReadFile(legacy.backup)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := service.OpenBackup(backup)
	if err != nil {
		t.Fatalf("the backup does not open with the current key: %v", err)
	}
	inner, err := service.OpenDocument(outer)
	if err != nil || !bytes.Equal(inner, []byte(unnestedSnippetsBackup)) {
		t.Fatalf("nested backup = %q, %v; want the nested %q", inner, err, unnestedSnippetsBackup)
	}
}

func migrationRecords(t *testing.T, service *secret.Service) int {
	t.Helper()
	history, err := service.TransactionsForTest().History()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, record := range history {
		if record.Operation == operationMigrateKeyBoundArtifacts {
			count++
		}
	}
	return count
}

// newLegacySnippetsWorkspace は、Vault を作ったあとに旧バージョンのファイルを置き、
// ロックしたワークスペースを返す。控えの外側は outer の鍵で封じる。
func newLegacySnippetsWorkspace(t *testing.T, outer backupOuterKey) (string, legacySnippets) {
	t.Helper()
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return home, placeLegacySnippets(t, workspace, outer(t, home))
}

func TestUnlockMovesThePlaintextSnippetsAndTheirUnnestedBackupToTheCurrentForm(t *testing.T) {
	for name, outer := range backupOuterKeys {
		t.Run(name, func(t *testing.T) {
			home, legacy := newLegacySnippetsWorkspace(t, outer)

			service := reopenWithSnippets(t, home)
			if err := service.Unlock(passphrase); err != nil {
				t.Fatal(err)
			}
			assertCurrentForm(t, service, legacy)
			if got := migrationRecords(t, service); got != 1 {
				t.Fatalf("migration records = %d, want 1", got)
			}
		})
	}
}

func TestUnlockWritesNothingOnceTheFilesAreInTheCurrentForm(t *testing.T) {
	home, legacy := newLegacySnippetsWorkspace(t, theVaultKey)
	service := reopenWithSnippets(t, home)
	if err := service.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	migrated := map[string][]byte{}
	for _, path := range []string{legacy.document, legacy.backup} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		migrated[path] = body
	}

	service.Lock()
	if err := service.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	for path, want := range migrated {
		if body, err := os.ReadFile(path); err != nil || !bytes.Equal(body, want) {
			t.Errorf("%s was rewritten by the second unlock", path)
		}
	}
	if got := migrationRecords(t, service); got != 1 {
		t.Fatalf("migration records = %d, want 1", got)
	}
}

func TestChangingTheMasterPasswordAfterTheUnlockMigrationKeepsTheSnippets(t *testing.T) {
	for name, outer := range backupOuterKeys {
		t.Run(name, func(t *testing.T) {
			home, legacy := newLegacySnippetsWorkspace(t, outer)
			service := reopenWithSnippets(t, home)
			if err := service.Unlock(passphrase); err != nil {
				t.Fatal(err)
			}

			const next = "a new master password"
			if err := service.ChangeMasterPassword(context.Background(), passphrase, next); err != nil {
				t.Fatalf("ChangeMasterPassword after the migration = %v", err)
			}
			reopened := reopenWithSnippets(t, home)
			if err := reopened.Unlock(next); err != nil {
				t.Fatal(err)
			}
			assertCurrentForm(t, reopened, legacy)
		})
	}
}

// ロックを解除していないあいだに残った古い形（Vault を作ったあとのスニペットの
// 平文など）は、マスターパスワードの変更が封じ直す前に移す。
func TestChangingTheMasterPasswordMovesWhatNoUnlockHasMovedYet(t *testing.T) {
	for name, outer := range backupOuterKeys {
		t.Run(name, func(t *testing.T) {
			service, home := newService(t)
			if err := service.Initialise(passphrase); err != nil {
				t.Fatal(err)
			}
			workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
			if err != nil {
				t.Fatal(err)
			}
			registerSnippets(t, service, workspace)
			legacy := placeLegacySnippets(t, workspace, outer(t, home))

			const next = "a new master password"
			if err := service.ChangeMasterPassword(context.Background(), passphrase, next); err != nil {
				t.Fatalf("ChangeMasterPassword with files left by an older release = %v", err)
			}
			reopened := reopenWithSnippets(t, home)
			if err := reopened.Unlock(next); err != nil {
				t.Fatal(err)
			}
			assertCurrentForm(t, reopened, legacy)
		})
	}
}

// reportedMigrationFailures は、ロックを解除したときの移行の失敗として知らされたものを集める。
func reportedMigrationFailures(service *secret.Service) *[]error {
	reported := &[]error{}
	service.SetReportKeyBoundMigrationFailure(func(err error) { *reported = append(*reported, err) })
	return reported
}

// 移行に失敗しても、ロックの解除は止めない。移せなかったファイルは残して失敗を知らせ、
// 鍵を変える変更は何も書かずに断る。
func TestAFailedUnlockMigrationStillUnlocksAndLeavesTheFileAlone(t *testing.T) {
	home, legacy := newLegacySnippetsWorkspace(t, theVaultKey)
	broken := []byte("not the snippets document")
	acltest.WritePrivateFile(t, legacy.document, broken)

	service := reopenWithSnippets(t, home)
	reported := reportedMigrationFailures(service)
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("Unlock with a document the migration refuses = %v", err)
	}
	if !service.Unlocked() {
		t.Fatal("the vault stayed locked")
	}
	if len(*reported) != 1 {
		t.Fatalf("reported migration failures = %v, want the refused document", *reported)
	}
	if body, err := os.ReadFile(legacy.document); err != nil || !bytes.Equal(body, broken) {
		t.Fatalf("the refused document = %q, %v; want it untouched", body, err)
	}
	if err := service.ChangeMasterPassword(context.Background(), passphrase, "a new master password"); err == nil {
		t.Fatal("ChangeMasterPassword re-sealed a document that is neither sealed nor valid")
	}
	if err := reopenWithSnippets(t, home).Unlock(passphrase); err != nil {
		t.Fatalf("the refused change moved the vault away from the current password: %v", err)
	}
}

// sshc/snippets.json が平文だったころの Vault は schema 3 で、今のバージョンでは
// 開けずにリセットするしかない。リセットはスニペットと控えを失わない。
func TestResettingAVaultFromBeforeSnippetsWereSealedKeepsTheSnippets(t *testing.T) {
	for name, outer := range backupOuterKeys {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
				t.Fatal(err)
			}
			workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
			if err != nil {
				t.Fatal(err)
			}
			legacyVaultFile := legacyVault(t, passphrase)
			acltest.WritePrivateFile(t, vaultPath(home), legacyVaultFile)
			legacy := placeLegacySnippets(t, workspace, outer(t, home))

			service := reopenWithSnippets(t, home)
			if err := service.Unlock(passphrase); !errors.Is(err, secret.ErrOlderSchema) {
				t.Fatalf("Unlock = %v, want ErrOlderSchema", err)
			}
			if err := service.ResetUnsupported(passphrase); err != nil {
				t.Fatalf("ResetUnsupported with files left by an older release = %v", err)
			}
			assertCurrentForm(t, service, legacy)
		})
	}
}

// 未対応の Vault を控えから戻すときも、古い形を先に移してから封じ直す。今の Vault は
// 控えを残したときと別の世代の鍵で封じてあるので、どちらの外側もマスターパスワードで開く。
func TestRecoveringACompatibleBackupMovesThePlaintextSnippetsAndTheirUnnestedBackup(t *testing.T) {
	for name, outer := range backupOuterKeys {
		t.Run(name, func(t *testing.T) {
			service, workspace, manager := recoveryService(t)
			if err := service.Initialise(passphrase); err != nil {
				t.Fatal(err)
			}
			registerSnippets(t, service, workspace)
			home := filepath.Dir(workspace.Root())
			legacy := placeLegacySnippets(t, workspace, outer(t, home))
			// 今の Vault を開けない schema のものに置き換える。置き換える前の Vault は
			// この変更の控えに残り、復旧はそれを戻す。
			compatible, err := os.ReadFile(vaultPath(home))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Commit(storage.Request{
				Operation: "test.install-unsupported-vault",
				Changes: []storage.Change{{
					Path: vaultPath(home), Contents: legacyVault(t, passphrase),
					Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(compatible)},
				}},
			}); err != nil {
				t.Fatal(err)
			}
			service.Lock()
			if err := service.Unlock(passphrase); !errors.Is(err, secret.ErrOlderSchema) {
				t.Fatalf("Unlock = %v, want ErrOlderSchema", err)
			}

			if err := service.RecoverCompatibleBackup(passphrase); err != nil {
				t.Fatalf("RecoverCompatibleBackup with files left by an older release = %v", err)
			}
			assertCurrentForm(t, service, legacy)
		})
	}
}

// 移行の途中で落ちたら、次の起動がロックを解除する前に巻き戻す。移す前の平文は
// 失わず、次のロックの解除で移し直す。
func TestStartupRollsBackAnInterruptedMigrationAndTheNextUnlockRedoesIt(t *testing.T) {
	home, legacy := newLegacySnippetsWorkspace(t, theVaultKey)
	faults := &rekeyFaultFileSystem{FileSystem: storage.OSFileSystem{}, failure: errors.New("injected migration failure")}
	faultyWorkspace, err := storage.NewWorkspace(faults, home)
	if err != nil {
		t.Fatal(err)
	}
	faults.targets = map[string]bool{legacy.document: true, legacy.backup: true}
	// 1 回目の失敗が適用を、2 回目が自動の巻き戻しを止め、プロセスが落ちたときと
	// 同じ保留の記録を残す。
	faults.failRenames = map[int]bool{2: true, 3: true}
	interrupted, _ := newSealedService(faultyWorkspace, time.Now)
	registerSnippets(t, interrupted, faultyWorkspace)
	reported := reportedMigrationFailures(interrupted)
	if err := interrupted.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	pending, err := interrupted.TransactionsForTest().Pending()
	if err != nil || len(pending) != 1 || pending[0].Operation != operationMigrateKeyBoundArtifacts {
		t.Fatalf("Pending after the interrupted migration = %#v, %v", pending, err)
	}
	if len(*reported) != 1 || !errors.Is((*reported)[0], faults.failure) {
		t.Fatalf("reported migration failures = %v, want the injected failure", *reported)
	}
	// 保留中の変更があって移せないのは、それが片付けば移せる失敗なので知らせない。
	interrupted.Lock()
	if err := interrupted.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if len(*reported) != 1 {
		t.Fatalf("reported migration failures = %v, want nothing for the pending change", *reported)
	}

	restarted := reopenWithSnippets(t, home)
	if err := restarted.AutoUnlock(); err != nil {
		t.Fatalf("AutoUnlock with an interrupted migration = %v", err)
	}
	if pending, err := restarted.TransactionsForTest().Pending(); err != nil || len(pending) != 0 {
		t.Fatalf("Pending after startup = %#v, %v", pending, err)
	}
	if body, err := os.ReadFile(legacy.document); err != nil || !bytes.Equal(body, []byte(plaintextSnippets)) {
		t.Fatalf("snippets after the rollback = %q, %v; want the plaintext back", body, err)
	}
	if err := restarted.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	assertCurrentForm(t, restarted, legacy)
}
