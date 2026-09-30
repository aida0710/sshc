package storage

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"time"
)

var (
	ErrUnknownTransaction = errors.New("no pending transaction with that identifier")
	ErrCannotComplete     = errors.New("staged contents are missing or altered")
	ErrCannotRollback     = errors.New("the transaction has crossed its durable commit point")
	// ErrRecoveryStateUnknown は、中断されたトランザクションの対象が、記録された
	// 変更前でも変更後でもない状態にあることを述べる。復旧はそこから先を推測しない。
	ErrRecoveryStateUnknown = errors.New("an interrupted transaction target no longer matches its recorded before or after state")
)

// PendingEntry は、中断されたトランザクションに含まれるファイルひとつ。
type PendingEntry struct {
	Path      string
	Target    string
	Action    string
	Committed bool
	HasBackup bool
	HasStaged bool
}

// Pending は、起動時に見つかった中断済みトランザクション。部分的な状態はそのまま
// 報告される。健全な結果として提示されることは決してない。
//
// CanComplete と CanRollback が両方 false になるのは、巻き戻せない変更を含む
// ときと、対象が記録のどちらの状態とも一致せず何が起きたか判別できないときで
// ある。後者は、中断されたトランザクションが触れるはずだったファイルを、外から
// 書き換えたときに起きる。
type Pending struct {
	ID          string
	Operation   string
	Status      string
	StartedAt   time.Time
	Committed   int
	Entries     []PendingEntry
	CanComplete bool
	CanRollback bool
}

// Pending は、中断されたトランザクションを古いものから順に列挙する。
func (m *Manager) Pending() ([]Pending, error) {
	records, err := m.readRecords(m.journalDirectory(), refuseOtherJournalVersions)
	if err != nil {
		return nil, err
	}
	pending := make([]Pending, 0, len(records))
	for _, record := range records {
		pending = append(pending, m.pendingItem(record))
	}
	return pending, nil
}

// pendingItem は、中断された記録ひとつを、対象の状態から数え直して報告の形にする。
func (m *Manager) pendingItem(record journalRecord) Pending {
	_, reconcileErr := m.reconcileRecord(&record)
	// 判別できないのは、その記録ひとつである。一覧そのものを失敗させると、
	// 無関係な記録も、履歴も、そしてこの記録を片付ける手段までもが同時に
	// 見えなくなる。呼び出し側はこの一覧で設定画面全体を組み立て、engine の
	// 起動時の自動ロック解除も Vault の記録を探すためにこれを読む。対象が
	// シンボリックリンクやディレクトリに置き換わった、上限を超えた、読めない
	// 権限になった、のように理由を問わず、判別できない記録はどちらの操作も
	// 提示しないまま並べる。Complete と Rollback は loadPending を通るので、
	// その記録に対しては引き続き同じ理由で拒否する。
	unresolved := reconcileErr != nil
	// 一覧は何も書き換えない。ここは呼び出し側が変更用の錠を持たずに
	// 呼ぶ経路であり、走っている最中のトランザクションの記録もそのまま読む。
	// 数え直した結果は報告に使うだけで、永続化するのは Complete と Rollback が
	// 通る loadPending だけである。
	item := Pending{
		ID:          record.ID,
		Operation:   record.Operation,
		Status:      record.Status,
		StartedAt:   record.StartedAt,
		Committed:   record.Committed,
		CanComplete: !unresolved && ((record.Status == statusStaged && !record.Atomic) || (record.Status == statusApplied && record.DiscardBackups)),
		CanRollback: !unresolved && record.Status != statusApplied,
	}
	for index, entry := range record.Entries {
		pendingEntry := PendingEntry{
			Path:      entry.Path,
			Target:    entry.Target,
			Action:    entry.Action,
			Committed: index < record.Committed,
			HasBackup: entry.Backup != "",
		}
		if pendingEntry.Committed && entry.checkReversible() != nil {
			item.CanRollback = false
		}
		if !pendingEntry.Committed && pendingEntry.Action == actionWrite {
			pendingEntry.HasStaged = m.stagedMatches(entry)
			if !pendingEntry.HasStaged {
				item.CanComplete = false
			}
		}
		item.Entries = append(item.Entries, pendingEntry)
	}
	return item
}

// Complete は、中断されたトランザクションを完了させる。検証すべきステージ済みの
// 内容を持つのは置き換えだけである。移動と削除は、その意図の全体をジャーナルの
// エントリに持っている。
func (m *Manager) Complete(identifier string) error {
	paths, err := m.complete(identifier)
	m.notifyRecovery(paths)
	return err
}

// complete は Complete の本体で、錠を持ったまま片付ける。ディスクに触れ始めた
// あとは、失敗しても記録が触れるパスを返す。途中まで進んだ分もメモリへ読み直させる
// ためである。
func (m *Manager) complete(identifier string) ([]string, error) {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()

	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return nil, err
	}
	paths := record.touchedPaths()
	// Atomic 記録は永続化文書とプロセス内状態（開いたパスワード Vault）を対応付ける。
	// callback 失敗後にディスク側だけを完了するとプロセス内状態が古くなるため、
	// 復旧時はロールバックする。新しい要求でディスクとメモリをまとめて更新できる。
	if record.Status == statusApplied && record.DiscardBackups {
		return paths, m.finishApplied(record, journalPath)
	}
	if record.Atomic || record.Status != statusStaged {
		return nil, ErrCannotComplete
	}
	for index := record.Committed; index < len(record.Entries); index++ {
		if record.Entries[index].Action != actionWrite {
			continue
		}
		if !m.stagedMatches(record.Entries[index]) {
			return nil, ErrCannotComplete
		}
	}
	if err := m.commitStaged(record); err != nil {
		return paths, err
	}
	return paths, m.finish(record, journalPath, statusCompleted)
}

// Rollback は、中断されたトランザクションがすでに変更したすべてのファイルを復元
// し、ステージ済みの内容を捨てる。すでにファイルを削除した、あるいは意図して
// バックアップを残さずに置き換えたトランザクションは、巻き戻せない。Rollback は、
// 実際には行っていない復旧を報告するのではなく、
// 拒否する。
func (m *Manager) Rollback(identifier string) error {
	paths, err := m.rollback(identifier)
	m.notifyRecovery(paths)
	return err
}

// rollback は Rollback の本体で、錠を持ったまま巻き戻す。complete と同じく、
// ディスクに触れ始めたあとは失敗しても記録が触れるパスを返す。
func (m *Manager) rollback(identifier string) ([]string, error) {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return nil, err
	}
	defer unlock()

	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return nil, err
	}
	if record.Status == statusApplied {
		return nil, ErrCannotRollback
	}
	return record.touchedPaths(), m.rollbackRecord(record, journalPath)
}

// rollbackRecord は CommitAtomic でも使用する。この経路では、対象の rename 後に
// SyncDir または journal の再書き込みが失敗し、メモリ上の記録が最新の永続記録より
// 先へ進む場合がある。そのため実行中プロセスが適用した操作はこの記録を基準にする。
func (m *Manager) rollbackRecord(record *journalRecord, journalPath string) error {
	for index := 0; index < record.Committed; index++ {
		if err := record.Entries[index].checkReversible(); err != nil {
			return err
		}
	}
	// バックアップを捨てる記録（マスターパスワード変更）は、以前の内容を封じずに
	// そのまま控えている。ReadBackup は封じた控えを開くので、ここでは使わない。
	readPrevious := m.ReadBackup
	if record.DiscardBackups {
		readPrevious = m.workspace.ReadTransactionFile
	}
	for index := record.Committed - 1; index >= 0; index-- {
		if err := m.rollbackEntry(record.Entries[index], readPrevious); err != nil {
			return err
		}
	}
	record.Committed = 0
	return m.finish(record, journalPath, statusRolledBack)
}

// rollbackEntry は、適用済みのエントリひとつを元に戻す。readPrevious は、置き換えや
// 削除の前の内容を控えから読む。
func (m *Manager) rollbackEntry(entry journalEntry, readPrevious func(path string) ([]byte, error)) error {
	switch {
	case entry.Action == actionMove:
		return m.rollbackMove(entry)
	case entry.Action == actionMakeDir:
		return m.rollbackMakeDirectory(entry)
	case entry.Action == actionRemoveDir:
		return m.rollbackRemoveDirectory(entry)
	case entry.HadPrevious:
		return m.restorePrevious(entry, readPrevious)
	default:
		return m.removeCreatedFile(entry)
	}
}

func (m *Manager) rollbackMove(entry journalEntry) error {
	fileSystem := m.workspace.FileSystem()
	if err := m.moveFile(entry.Target, entry.Path); err != nil {
		return err
	}
	if err := fileSystem.SyncDir(filepath.Dir(entry.Target)); err != nil {
		return err
	}
	return fileSystem.SyncDir(filepath.Dir(entry.Path))
}

// rollbackMakeDirectory は、このトランザクションが作ったディレクトリを、まだ空なら消す。
//
// もとからあったディレクトリは、このトランザクションが取り除いてよいもの
// ではない。取り消すのはこれが作ったものだけであり、しかもまだ空である
// 場合に限る。その後に何かが書き込まれているかもしれず、それを巻き戻しと
// 一緒に持っていけば、誰も触れてくれと頼んでいないものを削除することに
// なる。
func (m *Manager) rollbackMakeDirectory(entry journalEntry) error {
	if entry.HadPrevious {
		return nil
	}
	fileSystem := m.workspace.FileSystem()
	contents, readErr := fileSystem.ReadDir(entry.Path)
	if errors.Is(readErr, fs.ErrNotExist) {
		return nil
	}
	if readErr != nil {
		return readErr
	}
	if len(contents) > 0 {
		return nil
	}
	if err := fileSystem.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return fileSystem.SyncDir(filepath.Dir(entry.Path))
}

// rollbackRemoveDirectory は、取り除いた空のディレクトリを作り直す。取り除かれた
// 時点で空だったので、空のまま作り直せば失われたものがそのまま復元される。
func (m *Manager) rollbackRemoveDirectory(entry journalEntry) error {
	if err := m.workspace.EnsureDirectory(entry.Path); err != nil {
		return err
	}
	return m.workspace.FileSystem().SyncDir(filepath.Dir(entry.Path))
}

// restorePrevious は、置き換えか削除の前の内容を書き戻す。
func (m *Manager) restorePrevious(entry journalEntry, readPrevious func(path string) ([]byte, error)) error {
	if entry.noOpWrite() && entry.Backup == "" {
		// 変わっていないものを、作られなかった控えから戻す必要はない。
		return nil
	}
	if entry.sameContentsWrite() && entry.Backup == "" {
		contents, readErr := m.workspace.ReadTransactionFile(entry.Path)
		if readErr != nil {
			return readErr
		}
		writeErr := m.writeFile(entry.Path, contents, fs.FileMode(entry.PreviousMode))
		clear(contents)
		return writeErr
	}
	contents, readErr := readPrevious(entry.Backup)
	if readErr != nil {
		return readErr
	}
	restoreMode := entry.Mode
	if entry.Action == actionWrite {
		restoreMode = entry.PreviousMode
	}
	writeErr := m.writeFile(entry.Path, contents, fs.FileMode(restoreMode))
	// 復元したのは秘密鍵かもしれない。書き終えた控えは残さない。
	clear(contents)
	return writeErr
}

// removeCreatedFile は、このトランザクションが新しく作ったファイルを消す。
func (m *Manager) removeCreatedFile(entry journalEntry) error {
	fileSystem := m.workspace.FileSystem()
	if err := fileSystem.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return fileSystem.SyncDir(filepath.Dir(entry.Path))
}

func (m *Manager) stagedMatches(entry journalEntry) bool {
	if entry.Temp == "" {
		return false
	}
	info, err := m.workspace.FileSystem().Lstat(entry.Temp)
	if err != nil || !m.ownerModesMatch(info.Mode(), fs.FileMode(entry.Mode)) {
		return false
	}
	contents, err := ReadFileLimited(m.workspace.FileSystem(), entry.Temp, m.workspace.TransactionFileLimit(entry.Path))
	if err != nil {
		return false
	}
	digest := Digest(contents)
	clear(contents)
	return digest == entry.Digest
}

func (m *Manager) loadPending(identifier string) (*journalRecord, string, error) {
	if !validJournalIdentifier(identifier) {
		return nil, "", ErrUnknownTransaction
	}
	journalPath := filepath.Join(m.journalDirectory(), identifier+".json")
	contents, err := ReadFileLimited(m.workspace.FileSystem(), journalPath, maxRecordBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", ErrUnknownTransaction
	}
	if err != nil {
		return nil, "", err
	}
	var record journalRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		clear(contents)
		return nil, "", err
	}
	clear(contents)
	if err := m.validateLoadedJournalRecord(record, filepath.Base(journalPath), m.journalDirectory()); err != nil {
		return nil, "", err
	}
	if changed, err := m.reconcileRecord(&record); err != nil {
		return nil, "", err
	} else if changed {
		if err := m.validateLoadedJournalRecord(record, filepath.Base(journalPath), m.journalDirectory()); err != nil {
			return nil, "", err
		}
		if err := m.writeRecord(journalPath, record); err != nil {
			return nil, "", err
		}
	}
	return &record, journalPath, nil
}

// reconcileRecord は各対象の現在状態から、中断されたトランザクションの適用済み範囲を求める。
//
// commitStaged は進捗をエントリごとには書かず、失敗したときにだけ書き残す。そのため
// 永続化した Committed はファイルシステムより遅れることがあり（途中でプロセスが止まれば
// ステージしたときの 0 のまま）、途中で失敗したロールバックでは逆に先へ進むことがある。
// カウンターだけを信頼すると未処理の操作を処理済みと誤認するため、Pending、Complete、
// Rollback で記録を使う前に対象を観測して再計算する。
//
// 非 atomic の staging 記録は対象外とする。commitStaged に達しておらず、対象は未変更である。
// 進捗はディレクトリ作成だけで、これは永続文書に記録しない。
func (m *Manager) reconcileRecord(record *journalRecord) (bool, error) {
	if record.Status == statusCompleted || record.Status == statusRolledBack {
		return false, nil
	}
	statusChanged := false
	if record.Status == statusApplied {
		allApplied := true
		for _, entry := range record.Entries {
			evidence, err := m.entryEvidence(entry)
			if err != nil {
				return false, err
			}
			if evidence == evidenceUnapplied {
				allApplied = false
			}
		}
		if allApplied {
			return false, nil
		}
		// A failed durability write can leave an applied marker visible while the
		// same process has already begun rollback. Never finalize that mixed state
		// forward: return it to the rollback-capable staged state and reconstruct
		// its real prefix below.
		record.Status = statusStaged
		statusChanged = true
	}
	if !record.Atomic && record.Status != statusStaged {
		return false, nil
	}
	// 証拠を持つエントリだけが、本当の境界を両側から挟み込む。commitStaged は
	// 先頭から順に適用するので、適用済みの証拠は境界がその先にあることを言い、
	// 未適用の証拠は境界がその手前にあることを言う。
	lowest := 0
	highest := len(record.Entries)
	for index, entry := range record.Entries {
		evidence, err := m.entryEvidence(entry)
		if err != nil {
			return false, err
		}
		switch evidence {
		case evidenceApplied:
			if index+1 > lowest {
				lowest = index + 1
			}
		case evidenceUnapplied:
			if index < highest {
				highest = index
			}
		}
	}
	if lowest > highest {
		// 適用済みの証拠が未適用の証拠より後ろにある。順に進む書き手も、逆順に
		// 戻す巻き戻しも、この形は作らない。
		return false, ErrRecoveryStateUnknown
	}
	// 証拠を持たないエントリは境界の内側に数える。対象の姿はどちらに数えても
	// 変わらないが、外側に置くと、実際には適用済みで一時ファイルを使い切った
	// 書き込みが「未コミットなのにステージが無い」形になり、完了させられなくなる。
	committed := highest

	// 数え直すのは進捗だけである。ステージ済みファイルを手放すのは finish の
	// 仕事にしてある。ここで消すと、変更用の錠を持たない一覧の呼び出しが、
	// 走っている最中のトランザクションの一時ファイルを消せてしまう。
	changed := statusChanged || record.Committed != committed
	record.Committed = committed
	return changed, nil
}

// entryEvidence は、ひとつのエントリについて対象から読み取れる証拠。
type entryEvidence uint8

const (
	// evidenceUnapplied と evidenceApplied は、対象が記録された変更前・変更後の
	// どちらであるかを実際に見分けられた場合である。
	evidenceUnapplied entryEvidence = iota
	evidenceApplied
	// evidenceNone は、変更前と変更後が同じ姿をしていて、対象が何も語らない場合。
	// 内容の変わらない書き込みと、既にあったディレクトリの作成がこれにあたる。
	// ここを「適用済み」と読んではならない。直前が未適用のとき、ありもしない
	// 矛盾を作り出し、その記録は Pending も Complete も Rollback も永久に
	// 受け付けなくなる。
	evidenceNone
)

// entryApplied はエントリの対象変更が適用済みかを返す。記録された変更前・変更後の
// どちらでもない状態は推測せず拒否する。復旧の両方向がこの判定に依存するためである。
func (m *Manager) entryEvidence(entry journalEntry) (entryEvidence, error) {
	switch entry.Action {
	case actionMakeDir:
		if entry.HadPrevious {
			// もとからあったディレクトリは、作る前も作った後も同じように在る。
			return evidenceNone, nil
		}
		present, err := m.directoryPresent(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		return appliedWhen(present), nil
	case actionRemoveDir:
		present, err := m.directoryPresent(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		return appliedWhen(!present), nil
	case actionWrite:
		if entry.sameContentsWrite() && m.ownerModesMatch(fs.FileMode(entry.Mode), fs.FileMode(entry.PreviousMode)) {
			// 対象は書く前も書いた後も同じ姿である。何も語らない。実行ビットを
			// 読み返せない FileSystem では、権限だけを変える書き込みもこれにあたる。
			return evidenceNone, nil
		}
		digest, mode, exists, err := m.targetFileState(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		switch {
		case !exists && !entry.HadPrevious:
			return evidenceUnapplied, nil
		case exists && digest == entry.Digest && m.ownerModesMatch(mode, fs.FileMode(entry.Mode)):
			return evidenceApplied, nil
		case exists && entry.HadPrevious && digest == entry.PreviousDigest && m.ownerModesMatch(mode, fs.FileMode(entry.PreviousMode)):
			return evidenceUnapplied, nil
		}
		return evidenceNone, ErrRecoveryStateUnknown
	case actionMove:
		// 移動はバイト列をひとつしか持たない。したがって、どちらの側にそれがあるかが
		// 適用済みかどうかそのものである。
		source, sourceExists, err := m.targetDigest(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		target, targetExists, err := m.targetDigest(entry.Target)
		if err != nil {
			return evidenceNone, err
		}
		switch {
		case !sourceExists && targetExists && target == entry.Digest:
			return evidenceApplied, nil
		case sourceExists && !targetExists && source == entry.Digest:
			return evidenceUnapplied, nil
		}
		return evidenceNone, ErrRecoveryStateUnknown
	case actionRemove:
		digest, exists, err := m.targetDigest(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		switch {
		case !exists:
			return evidenceApplied, nil
		case digest == entry.Digest:
			return evidenceUnapplied, nil
		}
		return evidenceNone, ErrRecoveryStateUnknown
	}
	return evidenceNone, invalidJournal("entry action cannot be recovered")
}

func appliedWhen(applied bool) entryEvidence {
	if applied {
		return evidenceApplied
	}
	return evidenceUnapplied
}

func (m *Manager) directoryPresent(path string) (bool, error) {
	info, err := m.workspace.FileSystem().Lstat(path)
	switch {
	case err == nil && info.IsDir():
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	}
	return false, ErrRecoveryStateUnknown
}

// targetDigest はハッシュを取り終えたバイト列をゼロで埋める。復旧が読むのは、
// 秘密鍵かもしれないファイルそのものだからである。
func (m *Manager) targetDigest(path string) (string, bool, error) {
	digest, _, exists, err := m.targetFileState(path)
	return digest, exists, err
}

func (m *Manager) targetFileState(path string) (string, fs.FileMode, bool, error) {
	info, err := m.workspace.FileSystem().Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	contents, err := m.workspace.ReadTransactionFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	digest := Digest(contents)
	clear(contents)
	return digest, info.Mode().Perm() & 0o700, true, nil
}
