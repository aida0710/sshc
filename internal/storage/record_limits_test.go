package storage

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitRefusesMoreEntriesThanTheJournalCanRecordBeforeWritingAnything(t *testing.T) {
	manager, workspace := newTestManager(t)
	changes := make([]Change, 0, maxTransactionEntries+1)
	for index := range maxTransactionEntries + 1 {
		changes = append(changes, Change{
			Path:     filepath.Join(workspace.Root(), "many", fmt.Sprintf("%05d.conf", index)),
			Contents: []byte("Host many\n"),
		})
	}

	request := Request{
		Operation:   "too.many",
		Directories: []DirectoryCreate{{Path: filepath.Join(workspace.Root(), "many")}},
		Changes:     changes,
	}
	if _, err := manager.Commit(request); !errors.Is(err, ErrTransactionTooLarge) {
		t.Fatalf("Commit = %v, want ErrTransactionTooLarge", err)
	}
	for _, directory := range []string{filepath.Join(workspace.Root(), "many"), filepath.Join(workspace.StateDir(), journalDirectoryName)} {
		if entries, err := os.ReadDir(directory); err == nil && len(entries) > 0 {
			t.Fatalf("%s holds %d entries after a refused commit", directory, len(entries))
		}
	}
}

func TestHistoryReadsARecordLargerThanAConfigurationFile(t *testing.T) {
	manager, workspace := newTestManager(t)
	target := writeWorkspaceFile(t, workspace, "config", "before\n", FilePermission)
	result, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{{
			Path: target, Contents: []byte("after\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// マスターパスワード変更のようにバックアップを数千件抱えたトランザクションの履歴を、
	// 1 件の記録を増やして作る。
	historyPath := filepath.Join(workspace.StateDir(), historyDirectoryName, result.ID+".json")
	records, err := manager.readRecords(filepath.Dir(historyPath), skipOtherJournalVersions)
	if err != nil || len(records) != 1 {
		t.Fatalf("history records = %d, %v", len(records), err)
	}
	record := records[0]
	template := record.Entries[0]
	for index := 1; len(record.Entries) < 3000; index++ {
		relative := filepath.Join("sshc", "backups", "rekeyed", fmt.Sprintf("%05d", index), "config")
		entry := template
		entry.Path = filepath.Join(workspace.Root(), relative)
		entry.Backup = filepath.Join(workspace.StateDir(), backupDirectoryName, record.ID, relative)
		record.Entries = append(record.Entries, entry)
	}
	record.Committed = len(record.Entries)
	if err := manager.writeRecord(historyPath, record); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(historyPath); err != nil || info.Size() <= MaxFileSize {
		t.Fatalf("history record = %v, %v; want a record larger than %d bytes", info, err, MaxFileSize)
	}

	history, err := manager.History()
	if err != nil {
		t.Fatalf("History = %v", err)
	}
	if len(history) != 1 || len(history[0].Paths) != 3000 {
		t.Fatalf("history = %d records", len(history))
	}
	if _, err := manager.Commit(Request{
		Operation: "config.save",
		Changes: []Change{{
			Path: target, Contents: []byte("again\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("after\n"))},
		}},
	}); err != nil {
		t.Fatalf("Commit after a large history record = %v", err)
	}
}

// エントリごとに記録全体を書き直すと、時間がエントリ数の 2 乗で伸び、バックアップを
// 数千件抱えるマスターパスワード変更が workspace のロックを分単位で持ち続ける。
func TestCommitWritesItsJournalRecordTheSameNumberOfTimesForAnyEntryCount(t *testing.T) {
	journalWrites := func(entries int) int {
		workspace := newTestWorkspace(t)
		changes := make([]Change, 0, entries)
		for index := range entries {
			path := writeWorkspaceFile(t, workspace, fmt.Sprintf("%02d.conf", index), "before\n", FilePermission)
			changes = append(changes, Change{
				Path: path, Contents: []byte("after\n"),
				Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
			})
		}
		journalDirectory := filepath.Join(workspace.StateDir(), journalDirectoryName)
		writes := 0
		workspace.fileSystem = faultyFileSystem{
			FileSystem: OSFileSystem{},
			failOn: func(operation, path string) error {
				if operation == "rename" && filepath.Dir(path) == journalDirectory {
					writes++
				}
				return nil
			},
		}
		manager := NewManager(workspace, fixedClock(), bytes.NewReader(bytes.Repeat([]byte{0x5b}, 4096)))
		if _, err := manager.Commit(Request{Operation: "config.save", Changes: changes}); err != nil {
			t.Fatal(err)
		}
		return writes
	}
	if one, many := journalWrites(1), journalWrites(16); one != many {
		t.Fatalf("journal record written %d times for 1 entry and %d times for 16 entries; want the same", one, many)
	}
}

// 中断した大きな変更の保留記録は、設定ファイルの読み込み上限（MaxFileSize）を超えても
// 読めなければならない。読めないと、保留の一覧も完了も失敗し続け、engine の起動時の
// 自動ロック解除も止まる。
func TestAnInterruptedTransactionLargerThanAConfigurationFileCanBeListedAndCompleted(t *testing.T) {
	workspace := newTestWorkspace(t)
	// パスを長くして、少ない件数で記録を 1 MiB より大きくする。
	directory := strings.Repeat("d", 200)
	const entries = 1500
	changes := make([]Change, 0, entries)
	for index := range entries {
		path := writeWorkspaceFile(t, workspace, filepath.Join(directory, fmt.Sprintf("%04d.conf", index)), "before\n", FilePermission)
		changes = append(changes, Change{
			Path: path, Contents: []byte("after\n"),
			Precondition: Precondition{Exists: true, Digest: Digest([]byte("before\n"))},
		})
	}
	id := commitWithStaleJournal(t, workspace, 0x5c, Request{Operation: "config.save", Changes: changes})
	journalPath := filepath.Join(workspace.StateDir(), journalDirectoryName, id+".json")
	if info, err := os.Stat(journalPath); err != nil || info.Size() <= MaxFileSize {
		t.Fatalf("pending record = %v, %v; want a record larger than %d bytes", info, err, MaxFileSize)
	}

	restarted := restartedManager(t, workspace)
	reconciledPending(t, restarted, id, 1)
	if err := restarted.Complete(id); err != nil {
		t.Fatalf("Complete = %v", err)
	}
	for _, change := range changes {
		assertFileContents(t, change.Path, "after\n")
	}
	if pending, err := restarted.Pending(); err != nil || len(pending) != 0 {
		t.Fatalf("Pending after Complete = %#v, %v", pending, err)
	}
}

// 読む側の上限を超える記録は、書く前に断る。書けてしまうと、以後その記録を誰も読めない。
func TestAJournalRecordLargerThanTheReadLimitIsRefusedBeforeItIsWritten(t *testing.T) {
	manager, workspace := newTestManager(t)
	journalDirectory := filepath.Join(workspace.StateDir(), journalDirectoryName)
	if err := workspace.EnsureDirectory(journalDirectory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journalDirectory, "20260805T120000.000-5d5d5d5d.json")
	record := journalRecord{
		ID: "20260805T120000.000-5d5d5d5d", Version: journalVersion, Operation: "config.save", Status: statusStaging,
		Entries: []journalEntry{{Action: actionWrite, Path: filepath.Join(workspace.Root(), strings.Repeat("p", maxRecordBytes))}},
	}
	if err := manager.writeRecord(path, record); !errors.Is(err, ErrTransactionTooLarge) {
		t.Fatalf("writeRecord = %v, want ErrTransactionTooLarge", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused record exists: %v", err)
	}
}
