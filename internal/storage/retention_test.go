package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// retentionTestStart は、newTestManager の時計が最初に返す時刻の直前。
var retentionTestStart = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

// writeFinishedRecordFixture は、started に始めて完了した変更の記録と、その控えを置き、
// 記録の ID を返す。
func writeFinishedRecordFixture(t *testing.T, manager *Manager, started time.Time, index int) string {
	t.Helper()
	identifier := fmt.Sprintf("%s-%08x", started.UTC().Format(journalIdentifierTimeLayout), index)
	finished := started.UTC().Add(time.Second)
	record := journalRecord{
		ID: identifier, Version: journalVersion, Operation: "config.save", Status: statusCompleted,
		StartedAt: started.UTC(), FinishedAt: &finished, Committed: 1,
		Entries: []journalEntry{{Action: actionNote, Path: filepath.Join(manager.workspace.Root(), "config")}},
	}
	if err := manager.workspace.EnsureDirectory(manager.historyDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeRecord(filepath.Join(manager.historyDirectory(), identifier+".json"), record); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, manager.workspace, filepath.Join("sshc", backupDirectoryName, identifier, "config"), "Host before\n", 0o600)
	return identifier
}

// commitAChange は、設定を書き換えて控えを残す変更を完了させ、その記録の ID を返す。
func commitAChange(t *testing.T, manager *Manager) string {
	t.Helper()
	const previous = "Host before\n"
	path := writeWorkspaceFile(t, manager.workspace, "config", previous, 0o600)
	result, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{{
			Path: path, Contents: []byte("Host after\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte(previous))},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.ID
}

// assertRecordKept と assertRecordRemoved は、記録と控えの両方を見る。
func assertRecordKept(t *testing.T, manager *Manager, identifier string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(manager.historyDirectory(), identifier+".json"),
		filepath.Join(manager.backupsDirectory(), identifier),
	} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was removed: %v", path, err)
		}
	}
}

func assertRecordRemoved(t *testing.T, manager *Manager, identifier string) {
	t.Helper()
	assertMissing(t, filepath.Join(manager.historyDirectory(), identifier+".json"))
	assertMissing(t, filepath.Join(manager.backupsDirectory(), identifier))
}

// fillBackup は、記録の控えにファイルを足し、合わせて files 件にする。
func fillBackup(t *testing.T, manager *Manager, identifier string, files int) {
	t.Helper()
	directory := filepath.Join(manager.backupsDirectory(), identifier)
	// writeFinishedRecordFixture が config を 1 件置いてある。
	for index := 1; index < files; index++ {
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("key-%04d", index)), []byte("key\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// backupFilesLeft は、backups/ の下に残っているファイルの数を返す。
func backupFilesLeft(t *testing.T, manager *Manager) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(manager.backupsDirectory(), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// マスターパスワードの変更は、残した控えをすべて 1 回の変更で暗号化し直す。控えの上限は、
// 1 回の変更の上限より小さくする。
func TestTheRetainedBackupFilesFitInOneChange(t *testing.T) {
	if RetainedBackupFiles >= maxTransactionEntries {
		t.Fatalf("RetainedBackupFiles = %d reaches the limit of %d entries in one change",
			RetainedBackupFiles, maxTransactionEntries)
	}
}

func TestRecordsWithManyBackupsKeepTheBackupFilesUnderTheLimit(t *testing.T) {
	manager, _ := newTestManager(t)
	// 1 件で上限の 4 分の 1 を少し超える控えを持つ記録が続く。新しい方から 3 件までは
	// 上限に収まり、4 件目で超える。
	perRecord := RetainedBackupFiles/4 + 1
	identifiers := make([]string, 5)
	for index := range identifiers {
		identifiers[index] = writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-time.Hour+time.Duration(index)*time.Second), index)
		fillBackup(t, manager, identifiers[index], perRecord)
	}

	latest := commitAChange(t, manager)

	if left := backupFilesLeft(t, manager); left > RetainedBackupFiles {
		t.Fatalf("%d backup files are left, over the limit of %d", left, RetainedBackupFiles)
	}
	for _, identifier := range []string{latest, identifiers[4], identifiers[3], identifiers[2]} {
		assertRecordKept(t, manager, identifier)
	}
	for _, identifier := range identifiers[:2] {
		assertRecordRemoved(t, manager, identifier)
	}
}

// フォルダの削除のように 1 回で多くのファイルを書いた直後の変更は、控えが上限を超えて
// いても戻せるよう残す。それより古い記録は消す。
func TestTheNewestRecordIsKeptEvenWhenItsBackupsExceedTheLimit(t *testing.T) {
	manager, _ := newTestManager(t)
	older := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-2*time.Hour), 1)
	newest := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-time.Hour), 2)
	fillBackup(t, manager, newest, RetainedBackupFiles+1)

	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}

	assertRecordKept(t, manager, newest)
	assertRecordRemoved(t, manager, older)
}

func TestFinishingAChangeKeepsOnlyTheNewestRetainedRecords(t *testing.T) {
	manager, _ := newTestManager(t)
	identifiers := make([]string, RetainedHistoryRecords)
	for index := range identifiers {
		identifiers[index] = writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-time.Hour+time.Duration(index)*time.Second), index)
	}

	latest := commitAChange(t, manager)

	assertRecordRemoved(t, manager, identifiers[0])
	for _, identifier := range append(identifiers[1:], latest) {
		assertRecordKept(t, manager, identifier)
	}
	history, err := manager.History()
	if err != nil || len(history) != RetainedHistoryRecords {
		t.Fatalf("history = %d records, %v; want %d", len(history), err, RetainedHistoryRecords)
	}
}

func TestFinishingAChangeRemovesRecordsOlderThanTheRetentionPeriod(t *testing.T) {
	manager, _ := newTestManager(t)
	expired := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	kept := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod+time.Hour), 2)

	commitAChange(t, manager)

	assertRecordRemoved(t, manager, expired)
	assertRecordKept(t, manager, kept)
}

func TestPruneHistoryRemovesExpiredRecordsWithoutAChange(t *testing.T) {
	manager, _ := newTestManager(t)
	expired := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	// 記録を失った控えも、同じ規則で消す。
	orphan := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-2*time.Hour), 2)
	if err := os.Remove(filepath.Join(manager.historyDirectory(), orphan+".json")); err != nil {
		t.Fatal(err)
	}

	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}

	assertRecordRemoved(t, manager, expired)
	assertRecordRemoved(t, manager, orphan)
}

func TestPruningLeavesAnInterruptedChangeAndItsBackupsAlone(t *testing.T) {
	manager, workspace := newTestManager(t)
	// 中断した変更は、期間を過ぎていても、完了か取り消しで片付くまで残す。
	started := retentionTestStart.Add(-HistoryRetentionPeriod - time.Hour)
	interrupted := fmt.Sprintf("%s-%08x", started.Format(journalIdentifierTimeLayout), 0)
	journal := writeWorkspaceFile(t, workspace, filepath.Join("sshc", journalDirectoryName, interrupted+".json"), "{}\n", 0o600)
	backup := writeWorkspaceFile(t, workspace, filepath.Join("sshc", backupDirectoryName, interrupted, "config"), "Host before\n", 0o600)
	// 中断した変更は数えないので、完了した記録は上限までそのまま残る。
	finished := make([]string, RetainedHistoryRecords)
	for index := range finished {
		finished[index] = writeFinishedRecordFixture(t, manager, retentionTestStart.Add(time.Duration(-index)*time.Second), index+1)
	}

	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{journal, backup} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s of the interrupted change was removed: %v", path, err)
		}
	}
	for _, identifier := range finished {
		assertRecordKept(t, manager, identifier)
	}
}

func TestAChangeFinishesEvenWhenPruningFails(t *testing.T) {
	manager, workspace := newTestManager(t)
	expired := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	expiredRecord := filepath.Join(manager.historyDirectory(), expired+".json")
	failure := errors.New("injected removal failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "remove" && path == expiredRecord {
				return failure
			}
			return nil
		},
	}

	latest := commitAChange(t, manager)
	assertRecordKept(t, manager, latest)
	assertRecordKept(t, manager, expired)

	workspace.fileSystem = OSFileSystem{}
	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}
	assertRecordRemoved(t, manager, expired)
}

// 消せない記録が 1 件あっても、それより古い記録は消す。一覧は新しい順なので、そこで
// 止まると古い記録が毎回残り続ける。
func TestPruningGoesOnPastARecordItCannotRemove(t *testing.T) {
	manager, workspace := newTestManager(t)
	stuck := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	older := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-2*time.Hour), 2)
	stuckRecord := filepath.Join(manager.historyDirectory(), stuck+".json")
	failure := errors.New("injected removal failure")
	workspace.fileSystem = faultyFileSystem{
		FileSystem: OSFileSystem{},
		failOn: func(operation, path string) error {
			if operation == "remove" && path == stuckRecord {
				return failure
			}
			return nil
		},
	}

	if err := manager.PruneHistory(); !errors.Is(err, failure) {
		t.Fatalf("PruneHistory = %v, want the removal failure", err)
	}

	assertRecordRemoved(t, manager, older)
	assertRecordKept(t, manager, stuck)
}

// 90 日より前に始めて中断していた変更を完了させても、同じ完了の中で記録と控えを消さない。
// 次の片付けでは、ほかの記録と同じ規則で消える。
func TestCompletingAnOldInterruptedChangeKeepsItsRecordUntilTheNextPruning(t *testing.T) {
	_, workspace, _, _ := interruptedCommit(t)
	later := NewManager(workspace, func() time.Time {
		return retentionTestStart.Add(HistoryRetentionPeriod + 24*time.Hour)
	}, bytes.NewReader(bytes.Repeat([]byte{0x6b}, 4096)))
	later.Seal = sealForTest
	later.Unseal = unsealForTest
	pending, err := later.Pending()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending = %#v, %v", pending, err)
	}

	if err := later.Complete(pending[0].ID); err != nil {
		t.Fatal(err)
	}
	assertRecordKept(t, later, pending[0].ID)

	if err := later.PruneHistory(); err != nil {
		t.Fatal(err)
	}
	assertRecordRemoved(t, later, pending[0].ID)
}

func TestPruningRemovesABackupDirectoryThatIsALinkWithoutFollowingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege that Windows runners do not grant")
	}
	manager, _ := newTestManager(t)
	expired := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	backup := filepath.Join(manager.backupsDirectory(), expired)
	if err := os.RemoveAll(backup); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	kept := filepath.Join(outside, "config")
	if err := os.WriteFile(kept, []byte("outside the backups\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, backup); err != nil {
		t.Fatal(err)
	}

	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}

	assertRecordRemoved(t, manager, expired)
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("a file the link pointed at was removed: %v", err)
	}
}

func TestPruningRemovesALinkInsideABackupWithoutFollowingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege that Windows runners do not grant")
	}
	manager, _ := newTestManager(t)
	expired := writeFinishedRecordFixture(t, manager, retentionTestStart.Add(-HistoryRetentionPeriod-time.Hour), 1)
	outside := t.TempDir()
	kept := filepath.Join(outside, "kept")
	if err := os.WriteFile(kept, []byte("outside the backups\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(manager.backupsDirectory(), expired, "link")); err != nil {
		t.Fatal(err)
	}

	if err := manager.PruneHistory(); err != nil {
		t.Fatal(err)
	}

	assertRecordRemoved(t, manager, expired)
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("a file the link pointed at was removed: %v", err)
	}
}
