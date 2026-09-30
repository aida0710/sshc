package storage

import (
	"path/filepath"
	"time"
)

// HistoryRecord は完了したトランザクション。パスとハッシュだけを保持し、ファイルの
// 内容を保存することはない。保持の上限（RetainedHistoryRecords、
// HistoryRetentionPeriod）を超えた記録は、その控えとともに消える。
type HistoryRecord struct {
	ID         string
	Operation  string
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	Paths      []string
	BackupDir  string
}

// History は完了したトランザクションを、新しいものから順に返す。journalVersion と
// 違う版の記録は含めない。
func (m *Manager) History() ([]HistoryRecord, error) {
	records, err := m.readRecords(m.historyDirectory(), skipOtherJournalVersions)
	if err != nil {
		return nil, err
	}
	history := make([]HistoryRecord, 0, len(records))
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		item := HistoryRecord{
			ID:        record.ID,
			Operation: record.Operation,
			Status:    record.Status,
			StartedAt: record.StartedAt,
			BackupDir: filepath.Join(m.workspace.StateDir(), backupDirectoryName, record.ID),
		}
		if record.FinishedAt != nil {
			item.FinishedAt = *record.FinishedAt
		}
		for _, entry := range record.Entries {
			item.Paths = append(item.Paths, entry.Path)
		}
		history = append(history, item)
	}
	return history, nil
}

// Note は、ファイルを変えなかった完了済みの操作を記録する。秘密鍵の表示などが
// これにあたる。
//
// note にはステージされた内容もバックアップもジャーナルファイルもない。復旧すべき
// ものが何もないからだ。これがあるのは、履歴をアプリケーションが行ったことの完全な
// 記録にするためである。構造上、ファイルの内容を持ちようがない。保存するのは操作名、
// 時刻、関係したパスだけである。
func (m *Manager) Note(operation string, paths []string) (Result, error) {
	if operation == "" {
		return Result{}, ErrInvalidOperation
	}
	if len(paths) == 0 {
		return Result{}, ErrNoChanges
	}
	resolveEntries := func() ([]journalEntry, error) {
		entries := make([]journalEntry, 0, len(paths))
		claimed := newClaimedPaths(len(paths))
		for _, path := range paths {
			resolved, err := m.workspace.ResolveForWrite(path)
			if err != nil {
				return nil, err
			}
			if !claimed.claim(resolved) {
				return nil, ErrDuplicatePath
			}
			entries = append(entries, journalEntry{Action: actionNote, Path: resolved})
		}
		return entries, nil
	}
	// Reject lexical/structural errors before acquiring the persistent lock. The
	// same resolution is repeated under the lock so the preflight is not trusted
	// as a TOCTOU security decision.
	if _, err := resolveEntries(); err != nil {
		return Result{}, err
	}
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := m.ensureNoPendingTransaction(); err != nil {
		return Result{}, err
	}

	entries, err := resolveEntries()
	if err != nil {
		return Result{}, err
	}

	identifier, err := m.newIdentifier()
	if err != nil {
		return Result{}, err
	}
	historyDirectory := filepath.Join(m.workspace.StateDir(), historyDirectoryName)
	if err := m.workspace.EnsureDirectory(historyDirectory); err != nil {
		return Result{}, err
	}
	recorded := m.now().UTC()
	record := journalRecord{
		ID:         identifier,
		Version:    journalVersion,
		Operation:  operation,
		Status:     statusCompleted,
		StartedAt:  recorded,
		FinishedAt: &recorded,
		Committed:  len(entries),
		Entries:    entries,
	}
	if err := m.writeRecord(filepath.Join(historyDirectory, identifier+".json"), record); err != nil {
		return Result{}, err
	}
	return Result{ID: identifier}, nil
}
