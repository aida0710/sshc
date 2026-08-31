package storage

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrUnknownTransaction = errors.New("no pending transaction with that identifier")
	ErrCannotComplete     = errors.New("staged contents are missing or altered")
	ErrCannotRollback     = errors.New("the transaction has crossed its durable commit point")
	// ErrRecoveryStateUnknown は、中断されたトランザクションの対象が、記録された
	// 変更前でも変更後でもない状態にあることを述べる。復旧はそこから先を推測しない。
	ErrRecoveryStateUnknown = errors.New("an interrupted transaction target no longer matches its recorded before or after state")
	ErrInvalidJournal       = errors.New("invalid transaction journal")
)

var journalIdentifierPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{3}-[0-9a-f]{8}$`)

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

func (m *Manager) journalDirectory() string {
	return filepath.Join(m.workspace.StateDir(), journalDirectoryName)
}

func (m *Manager) historyDirectory() string {
	return filepath.Join(m.workspace.StateDir(), historyDirectoryName)
}

// Pending は、中断されたトランザクションを古いものから順に列挙する。
func (m *Manager) Pending() ([]Pending, error) {
	records, err := m.readRecords(m.journalDirectory())
	if err != nil {
		return nil, err
	}
	pending := make([]Pending, 0, len(records))
	for _, record := range records {
		_, reconcileErr := m.reconcileRecord(&record)
		// 判別できないのは、その記録ひとつである。一覧そのものを失敗させると、
		// 無関係な記録も、履歴も、そしてこの記録を片付ける手段までもが同時に
		// 見えなくなる。呼び出し側はこの一覧で設定画面全体を組み立てている。
		// 判別できない記録は、どちらの操作も提示しないまま並べる。Complete と
		// Rollback は、その記録に対しては引き続き同じ理由で拒否する。
		unresolved := errors.Is(reconcileErr, ErrRecoveryStateUnknown)
		if reconcileErr != nil && !unresolved {
			return nil, reconcileErr
		}
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
			switch {
			case pendingEntry.Committed && pendingEntry.Action == actionRemove && entry.NoBackup:
				item.CanRollback = false
			case pendingEntry.Committed && pendingEntry.Action == actionWrite && entry.HadPrevious && entry.NoBackup && !entry.sameContentsWrite():
				item.CanRollback = false
			case !pendingEntry.Committed && pendingEntry.Action == actionWrite:
				pendingEntry.HasStaged = m.stagedMatches(entry)
				if !pendingEntry.HasStaged {
					item.CanComplete = false
				}
			}
			item.Entries = append(item.Entries, pendingEntry)
		}
		pending = append(pending, item)
	}
	return pending, nil
}

// Complete は、中断されたトランザクションを完了させる。検証すべきステージ済みの
// 内容を持つのは置き換えだけである。移動と削除は、その意図の全体をジャーナルの
// エントリに持っている。
func (m *Manager) Complete(identifier string) error {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()

	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return err
	}
	// Atomic 記録は永続化文書とプロセス内状態（開いたパスワード Vault）を対応付ける。
	// callback 失敗後にディスク側だけを完了するとプロセス内状態が古くなるため、
	// 復旧時はロールバックする。新しい要求でディスクとメモリをまとめて更新できる。
	if record.Status == statusApplied && record.DiscardBackups {
		return m.finishApplied(record, journalPath)
	}
	if record.Atomic || record.Status != statusStaged {
		return ErrCannotComplete
	}
	for index := record.Committed; index < len(record.Entries); index++ {
		if record.Entries[index].Action != actionWrite {
			continue
		}
		if !m.stagedMatches(record.Entries[index]) {
			return ErrCannotComplete
		}
	}
	if err := m.commitStaged(record, journalPath); err != nil {
		return err
	}
	return m.finish(record, journalPath, statusCompleted)
}

// Rollback は、中断されたトランザクションがすでに変更したすべてのファイルを復元
// し、ステージ済みの内容を捨てる。すでにファイルを削除した、あるいは意図して
// バックアップを残さずに置き換えたトランザクションは、巻き戻せない。Rollback は、
// 実際には行っていない復旧を報告するのではなく、
// 拒否する。
func (m *Manager) Rollback(identifier string) error {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()

	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return err
	}
	if record.Status == statusApplied {
		return ErrCannotRollback
	}
	return m.rollbackRecord(record, journalPath)
}

// rollbackRecord は CommitAtomic でも使用する。この経路では、対象の rename 後に
// SyncDir または journal の再書き込みが失敗し、メモリ上の記録が最新の永続記録より
// 先へ進む場合がある。そのため実行中プロセスが適用した操作はこの記録を基準にする。
func (m *Manager) rollbackRecord(record *journalRecord, journalPath string) error {
	for index := 0; index < record.Committed; index++ {
		entry := record.Entries[index]
		// バックアップを残した削除は、置き換えと同じくらい可逆である。バイト列は
		// 世代ディレクトリにあり、モードはエントリにある。巻き戻せないのは、意図して
		// 何も残さなかったものだけである。
		if entry.Action == actionRemove && entry.NoBackup {
			return ErrIrreversibleRemoval
		}
		if entry.Action == actionWrite && entry.HadPrevious && entry.NoBackup && !entry.sameContentsWrite() {
			return ErrIrreversibleChange
		}
	}

	fileSystem := m.workspace.FileSystem()
	for index := record.Committed - 1; index >= 0; index-- {
		entry := record.Entries[index]
		if entry.Action == actionMove {
			if err := m.moveFile(entry.Target, entry.Path); err != nil {
				return err
			}
			if err := fileSystem.SyncDir(filepath.Dir(entry.Target)); err != nil {
				return err
			}
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
			continue
		}
		if entry.Action == actionMakeDir {
			// もとからあったディレクトリは、このトランザクションが取り除いてよいもの
			// ではない。取り消すのはこれが作ったものだけであり、しかもまだ空である
			// 場合に限る。その後に何かが書き込まれているかもしれず、それを巻き戻しと
			// 一緒に持っていけば、誰も触れてくれと頼んでいないものを削除することに
			// なる。
			if entry.HadPrevious {
				continue
			}
			contents, readErr := fileSystem.ReadDir(entry.Path)
			if errors.Is(readErr, fs.ErrNotExist) {
				continue
			}
			if readErr != nil {
				return readErr
			}
			if len(contents) > 0 {
				continue
			}
			if err := fileSystem.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
			continue
		}
		if entry.Action == actionRemoveDir {
			// 取り除かれた時点で空だったので、空のまま作り直せば失われたものが
			// そのまま復元される。
			if err := m.workspace.EnsureDirectory(entry.Path); err != nil {
				return err
			}
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
			continue
		}
		if entry.HadPrevious {
			if entry.noOpWrite() && entry.Backup == "" {
				// 変わっていないものを、作られなかった控えから戻す必要はない。
				continue
			}
			if entry.sameContentsWrite() && entry.Backup == "" {
				contents, readErr := fileSystem.ReadFile(entry.Path)
				if readErr != nil {
					return readErr
				}
				writeErr := m.writeFile(entry.Path, contents, fs.FileMode(entry.beforeMode()))
				zeroBytes(contents)
				if writeErr != nil {
					return writeErr
				}
				continue
			}
			var contents []byte
			var readErr error
			if record.DiscardBackups {
				contents, readErr = fileSystem.ReadFile(entry.Backup)
			} else {
				contents, readErr = m.ReadBackup(entry.Backup)
			}
			if readErr != nil {
				return readErr
			}
			restoreMode := entry.Mode
			if entry.Action == actionWrite {
				restoreMode = entry.beforeMode()
			}
			writeErr := m.writeFile(entry.Path, contents, fs.FileMode(restoreMode))
			// 復元したのは秘密鍵かもしれない。書き終えた控えは残さない。
			zeroBytes(contents)
			if writeErr != nil {
				return writeErr
			}
			continue
		}
		if err := fileSystem.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
			return err
		}
	}
	record.Committed = 0
	return m.finish(record, journalPath, statusRolledBack)
}

func (m *Manager) stagedMatches(entry journalEntry) bool {
	if entry.Temp == "" {
		return false
	}
	info, err := m.workspace.FileSystem().Lstat(entry.Temp)
	if err != nil || uint32(info.Mode().Perm()&0o700) != entry.Mode {
		return false
	}
	contents, err := m.workspace.FileSystem().ReadFile(entry.Temp)
	if err != nil {
		return false
	}
	digest := Digest(contents)
	zeroBytes(contents)
	return digest == entry.Digest
}

func (m *Manager) loadPending(identifier string) (*journalRecord, string, error) {
	if !validJournalIdentifier(identifier) {
		return nil, "", ErrUnknownTransaction
	}
	journalPath := filepath.Join(m.journalDirectory(), identifier+".json")
	contents, err := m.workspace.FileSystem().ReadFile(journalPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", ErrUnknownTransaction
	}
	if err != nil {
		return nil, "", err
	}
	var record journalRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		zeroBytes(contents)
		return nil, "", err
	}
	zeroBytes(contents)
	if err := migrateLoadedJournalRecord(&record); err != nil {
		return nil, "", err
	}
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
// 対象の変更は、それを記録する journal の再書き込みより先に行われる。そのため永続化した
// Committed はファイルシステムより遅れることがあり、途中で失敗したロールバックでは逆に
// 先へ進むことがある。カウンターだけを信頼すると未処理の操作を処理済みと誤認するため、
// Pending、Complete、Rollback で記録を使う前に対象を観測して再計算する。
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
		if entry.noOpWrite() {
			// 対象は書く前も書いた後も同じ姿である。何も語らない。
			return evidenceNone, nil
		}
		digest, mode, exists, err := m.targetFileState(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		switch {
		case !exists && !entry.HadPrevious:
			return evidenceUnapplied, nil
		case exists && digest == entry.Digest && uint32(mode) == entry.Mode:
			return evidenceApplied, nil
		case exists && entry.HadPrevious && digest == entry.PreviousDigest && uint32(mode) == entry.beforeMode():
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
	contents, err := m.workspace.FileSystem().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	digest := Digest(contents)
	zeroBytes(contents)
	return digest, info.Mode().Perm() & 0o700, true, nil
}

// readRecords は、ディレクトリ内のすべてのジャーナル文書を古い順に読み込む。
//
// 識別子はミリ秒までの UTC タイムスタンプで始まり、そのあとに乱数が続く。同じ
// ミリ秒に落ちた 2 件の間では、辞書順は時系列順ではない。接頭辞が一致するので、
// 順序を決めるのは乱数になる。そこで並べ替えは記録が保持しているナノ秒精度の
// StartedAt で行い、識別子は同時刻の決定的なタイブレークにだけ使う。
//
// 読み込み自体は名前順で行う。ここが時系列である必要はないが、壊れた文書に当たった
// ときに返るエラーが実行ごとに変わらないほうが調べやすい。
func (m *Manager) readRecords(directory string) ([]journalRecord, error) {
	entries, err := m.workspace.FileSystem().ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			if _, err := journalIdentifierFromName(entry.Name()); err != nil {
				return nil, err
			}
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	records := make([]journalRecord, 0, len(names))
	for _, name := range names {
		contents, readErr := m.workspace.FileSystem().ReadFile(filepath.Join(directory, name))
		if readErr != nil {
			return nil, readErr
		}
		var record journalRecord
		if unmarshalErr := json.Unmarshal(contents, &record); unmarshalErr != nil {
			zeroBytes(contents)
			return nil, unmarshalErr
		}
		zeroBytes(contents)
		if migrationErr := migrateLoadedJournalRecord(&record); migrationErr != nil {
			return nil, migrationErr
		}
		if validationErr := m.validateLoadedJournalRecord(record, name, directory); validationErr != nil {
			return nil, validationErr
		}
		records = append(records, record)
	}
	sort.SliceStable(records, func(i, j int) bool {
		if !records[i].StartedAt.Equal(records[j].StartedAt) {
			return records[i].StartedAt.Before(records[j].StartedAt)
		}
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func validJournalIdentifier(identifier string) bool {
	return journalIdentifierPattern.MatchString(identifier)
}

func journalIdentifierFromName(name string) (string, error) {
	if name != filepath.Base(name) || filepath.Ext(name) != ".json" {
		return "", invalidJournal("invalid document name")
	}
	identifier := strings.TrimSuffix(name, ".json")
	if !validJournalIdentifier(identifier) {
		return "", invalidJournal("invalid transaction identifier")
	}
	return identifier, nil
}

func invalidJournal(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidJournal, reason)
}

// migrateLoadedJournalRecord upgrades the released v1 contract in memory.
// In v1 a write had one Mode because its before and after modes were identical.
// v2 separates them so a mode-only write is recoverable after a crash.
func migrateLoadedJournalRecord(record *journalRecord) error {
	if record.Version == journalVersion {
		return nil
	}
	if record.Version != 1 {
		return invalidJournal("identity or version mismatch")
	}
	for index := range record.Entries {
		entry := &record.Entries[index]
		if entry.PreviousMode != 0 {
			return invalidJournal("v1 entry contains a v2 mode")
		}
		if entry.Action == actionWrite && entry.HadPrevious {
			entry.PreviousMode = entry.Mode
		}
	}
	record.Version = journalVersion
	return nil
}

func (m *Manager) validateLoadedJournalRecord(record journalRecord, name, directory string) error {
	identifier, err := journalIdentifierFromName(name)
	if err != nil {
		return err
	}
	if record.ID != identifier || record.Version != journalVersion {
		return invalidJournal("identity or version mismatch")
	}
	if record.Operation == "" || record.StartedAt.IsZero() || len(record.Entries) == 0 {
		return invalidJournal("missing required record fields")
	}
	if record.Committed < 0 || record.Committed > len(record.Entries) {
		return invalidJournal("committed count is out of bounds")
	}

	pending := sameJournalPath(directory, m.journalDirectory())
	history := sameJournalPath(directory, m.historyDirectory())
	if !pending && !history {
		return invalidJournal("unexpected journal directory")
	}
	if pending {
		if record.Status != statusStaging && record.Status != statusStaged && record.Status != statusApplied {
			return invalidJournal("unexpected pending status")
		}
	} else {
		if record.Status != statusCompleted && record.Status != statusRolledBack {
			return invalidJournal("unexpected history status")
		}
	}
	if record.Status == statusStaging || record.Status == statusStaged || record.Status == statusApplied {
		if record.FinishedAt != nil {
			return invalidJournal("unfinished record has a finish time")
		}
		if record.Status == statusApplied && (!record.Atomic || !record.DiscardBackups || record.Committed != len(record.Entries)) {
			return invalidJournal("applied record is not a complete discard-backup transaction")
		}
		if record.Status == statusStaging && !record.Atomic && record.Committed != 0 {
			return invalidJournal("non-atomic staging progress is not durable")
		}
	} else {
		if record.FinishedAt == nil || record.FinishedAt.Before(record.StartedAt) {
			return invalidJournal("invalid finish time")
		}
		if record.Status == statusCompleted && record.Committed != len(record.Entries) {
			return invalidJournal("completed record has incomplete progress")
		}
		if record.Status == statusRolledBack && record.Committed != 0 {
			return invalidJournal("rolled-back record has progress")
		}
	}

	noteEntries := 0
	claimed := make([]string, 0, len(record.Entries)*2)
	for index, entry := range record.Entries {
		if err := m.validateLoadedJournalEntry(record, entry, index, pending); err != nil {
			return err
		}
		if journalPathAlreadyClaimed(claimed, entry.Path) {
			return invalidJournal("duplicate entry path")
		}
		claimed = append(claimed, entry.Path)
		if entry.Target != "" {
			if journalPathAlreadyClaimed(claimed, entry.Target) {
				return invalidJournal("duplicate entry target")
			}
			claimed = append(claimed, entry.Target)
		}
		if entry.Action == actionNote {
			noteEntries++
		}
	}
	if noteEntries != 0 && (noteEntries != len(record.Entries) || !history || record.Status != statusCompleted || record.Atomic) {
		return invalidJournal("note entries require completed non-atomic history")
	}
	if record.DiscardBackups && !record.Atomic {
		return invalidJournal("discard-backup record is not atomic")
	}
	return nil
}

func (m *Manager) validateLoadedJournalEntry(record journalRecord, entry journalEntry, index int, pending bool) error {
	if entry.Action == "" {
		return invalidJournal("entry action is required")
	}
	if !m.validLoadedWorkspacePath(entry.Path) {
		return invalidJournal("entry path is outside the workspace")
	}
	if entry.Target != "" && !m.validLoadedWorkspacePath(entry.Target) {
		return invalidJournal("entry target is outside the workspace")
	}
	if entry.Temp != "" && !m.validLoadedWorkspacePath(entry.Temp) {
		return invalidJournal("entry temp is outside the workspace")
	}

	digestValid := validJournalDigest(entry.Digest)
	previousDigestValid := validJournalDigest(entry.PreviousDigest)
	switch entry.Action {
	case actionWrite:
		if entry.Target != "" || !validOwnerFileMode(entry.Mode) || !digestValid {
			return invalidJournal("invalid write entry")
		}
		if record.Atomic && entry.NoBackup {
			return invalidJournal("atomic write cannot omit its backup")
		}
		if entry.HadPrevious != previousDigestValid {
			return invalidJournal("write previous digest mismatch")
		}
		if (!entry.HadPrevious && entry.PreviousMode != 0) || !validOwnerFileMode(entry.PreviousMode) {
			return invalidJournal("write previous mode mismatch")
		}
		if entry.Temp != "" {
			prefix := temporaryPrefix + record.ID + "-"
			if filepath.Dir(entry.Temp) != filepath.Dir(entry.Path) || !strings.HasPrefix(filepath.Base(entry.Temp), prefix) {
				return invalidJournal("write temp is not the expected sibling")
			}
		}
		// 未コミットの書き込みがステージ済みファイルを持たないことはありうる。それを
		// 巻き戻して、その先で失敗した復旧は、対象を以前の内容に戻したまま一時ファイル
		// を消費し尽くしており、照合はその記録をそのまま書き戻すからだ。この状態の
		// エントリは何も引き起動しない。Complete は使う直前にステージ済みファイルを
		// すべて検証して拒否し、Pending は完了不可として報告するので、読み手は
		// 復旧そのものを立ち往生させる代わりにこれを受理する。
		// コミット済みの書き込みがステージ済みファイルの名前を残していることも
		// ありうる。rename が使い切ったのに進捗の書き換えが失敗した記録も、
		// 内容の変わらない書き込みを適用済み側に数えた記録も、この形になる。
		// これも何も引き起動しない。Complete はその添字より先からしか進まず、
		// finish が記録を履歴にする前に名前も対象も手放す。
		if record.Status == statusCompleted && entry.Temp != "" {
			return invalidJournal("completed write retains a staged path")
		}
		if err := m.validateLoadedBackup(record, entry, pending); err != nil {
			return err
		}
	case actionMove:
		if entry.Target == "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || !entry.HadPrevious ||
			entry.PreviousMode != 0 || !validOwnerFileMode(entry.Mode) || !digestValid || entry.PreviousDigest != entry.Digest {
			return invalidJournal("invalid move entry")
		}
		if record.Atomic {
			return invalidJournal("atomic record contains a move")
		}
	case actionRemove:
		if entry.Target != "" || entry.Temp != "" || !entry.HadPrevious || !validOwnerFileMode(entry.Mode) ||
			entry.PreviousMode != 0 || !digestValid || entry.PreviousDigest != entry.Digest {
			return invalidJournal("invalid remove entry")
		}
		if record.Atomic {
			return invalidJournal("atomic record contains a removal")
		}
		if err := m.validateLoadedBackup(record, entry, pending); err != nil {
			return err
		}
	case actionMakeDir:
		if entry.Target != "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup ||
			entry.Digest != "" || entry.PreviousDigest != "" || entry.PreviousMode != 0 || entry.Mode != uint32(DirectoryPermission) {
			return invalidJournal("invalid mkdir entry")
		}
	case actionRemoveDir:
		if entry.Target != "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || !entry.HadPrevious ||
			entry.Digest != "" || entry.PreviousDigest != "" || entry.PreviousMode != 0 || entry.Mode&^uint32(0o777) != 0 {
			return invalidJournal("invalid rmdir entry")
		}
		if record.Atomic {
			return invalidJournal("atomic record contains rmdir")
		}
	case actionNote:
		if entry.Target != "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || entry.HadPrevious ||
			entry.Digest != "" || entry.PreviousDigest != "" || entry.PreviousMode != 0 || entry.Mode != 0 {
			return invalidJournal("invalid note entry")
		}
	default:
		return invalidJournal("unknown entry action")
	}
	return nil
}

func validOwnerFileMode(mode uint32) bool {
	return mode&^uint32(0o700) == 0
}

func (m *Manager) validateLoadedBackup(record journalRecord, entry journalEntry, pending bool) error {
	expectsBackup := entry.HadPrevious && !entry.NoBackup
	if !expectsBackup {
		if entry.Backup != "" {
			return invalidJournal("unexpected backup path")
		}
		return nil
	}
	if entry.Backup == "" {
		if record.DiscardBackups && (record.Status == statusApplied || record.Status == statusCompleted || record.Status == statusRolledBack) {
			return nil
		}
		if (pending && record.Status == statusStaging) || record.Status == statusRolledBack {
			return nil
		}
		return invalidJournal("required backup path is missing")
	}
	relative, err := filepath.Rel(m.workspace.Root(), entry.Path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return invalidJournal("backup target is invalid")
	}
	expected := filepath.Join(m.workspace.StateDir(), backupDirectoryName, record.ID, relative)
	if !sameJournalPath(entry.Backup, expected) {
		return invalidJournal("backup path does not match its entry")
	}
	return nil
}

func (m *Manager) validLoadedWorkspacePath(path string) bool {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || sameJournalPath(path, m.workspace.Root()) {
		return false
	}
	return privateStateContains(m.workspace.Root(), path)
}

func validJournalDigest(digest string) bool {
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32
}

func sameJournalPath(first, second string) bool {
	return privateStateContains(first, second) && privateStateContains(second, first)
}

func journalPathAlreadyClaimed(claimed []string, candidate string) bool {
	for _, path := range claimed {
		if sameJournalPath(path, candidate) {
			return true
		}
	}
	return false
}
