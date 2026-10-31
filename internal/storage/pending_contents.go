package storage

import "errors"

// ErrNoPreviousCopy は、書き終えたファイルの変更前の中身が控えに無いことを言う。
// 控えを取らない置き換え（起動時の書き直しなど）で起きる。
var ErrNoPreviousCopy = errors.New("the previous contents were not kept")

// PendingContents は、中断した変更のファイルひとつについて、変更前と変更後の中身である。
// History で前と後を比べるために読む。中身を持つのは置き換えと削除だけで、移動と
// ディレクトリの作成・削除は Before も After も空のままにする。
type PendingContents struct {
	Path      string
	Target    string
	Action    string
	Committed bool
	Before    PendingSide
	After     PendingSide
}

// PendingSide は、変更前か変更後のどちらか一方である。Exists が false なら、その側には
// ファイルが無い（新しく作る、または消す）。Err が nil でなければ、中身を読めなかった。
type PendingSide struct {
	Exists   bool
	Contents []byte
	Err      error
}

// PendingContents は、中断した変更 identifier のファイルごとの変更前と変更後を読む。
//
// 記録も対象も書き換えない。変更用の錠を持って読むのは、読んでいるあいだに Complete や
// Rollback、ほかのプロセスの変更が対象を動かさないようにするためである。書き終えた範囲は
// Pending と同じく対象の状態から数え直す。
func (m *Manager) PendingContents(identifier string) ([]PendingContents, error) {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()

	record, _, err := m.readPendingRecord(identifier)
	if err != nil {
		return nil, err
	}
	if _, err := m.reconcileRecord(&record); err != nil {
		return nil, err
	}
	readPrevious := m.previousReader(&record)
	contents := make([]PendingContents, 0, len(record.Entries))
	for index, entry := range record.Entries {
		item := PendingContents{
			Path:      entry.Path,
			Target:    entry.Target,
			Action:    entry.Action,
			Committed: index < record.Committed,
		}
		switch entry.Action {
		case actionWrite:
			item.Before = m.beforeWrite(entry, item.Committed, readPrevious)
			item.After = m.afterWrite(entry, item.Committed)
		case actionRemove:
			item.Before = m.beforeRemove(entry, item.Committed, readPrevious)
		}
		contents = append(contents, item)
	}
	return contents, nil
}

// beforeWrite は、置き換えの前の中身を読む。書き終えていれば控えから、まだなら対象から読む。
func (m *Manager) beforeWrite(entry journalEntry, committed bool, readPrevious func(string) ([]byte, error)) PendingSide {
	if !entry.HadPrevious {
		return PendingSide{}
	}
	if !committed {
		return m.readSide(entry.Path)
	}
	if entry.Backup != "" {
		contents, err := readPrevious(entry.Backup)
		return PendingSide{Exists: true, Contents: contents, Err: err}
	}
	// 中身の変わらない置き換えは控えを取らない。前と後は同じバイト列である。
	if entry.sameContentsWrite() {
		return m.readSide(entry.Path)
	}
	return PendingSide{Exists: true, Err: ErrNoPreviousCopy}
}

// afterWrite は、置き換えたあとの中身を読む。書き終えていれば対象から、まだならステージした
// 一時ファイルから読む。一時ファイルが無いか記録と違えば、完了もできないので理由を返す。
func (m *Manager) afterWrite(entry journalEntry, committed bool) PendingSide {
	if committed {
		return m.readSide(entry.Path)
	}
	if !m.stagedMatches(entry) {
		return PendingSide{Exists: true, Err: ErrCannotComplete}
	}
	return m.readSide(entry.Temp)
}

// beforeRemove は、消す前の中身を読む。消し終えていれば控えから、まだなら対象から読む。
func (m *Manager) beforeRemove(entry journalEntry, committed bool, readPrevious func(string) ([]byte, error)) PendingSide {
	if !committed {
		return m.readSide(entry.Path)
	}
	if entry.Backup == "" {
		return PendingSide{Exists: true, Err: ErrNoPreviousCopy}
	}
	contents, err := readPrevious(entry.Backup)
	return PendingSide{Exists: true, Contents: contents, Err: err}
}

func (m *Manager) readSide(path string) PendingSide {
	contents, err := m.workspace.ReadTransactionFile(path)
	return PendingSide{Exists: true, Contents: contents, Err: err}
}
