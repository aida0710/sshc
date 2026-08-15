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
		changed, reconcileErr := m.reconcileRecord(&record)
		// 判別できないのは、その記録ひとつである。一覧そのものを失敗させると、
		// 無関係な記録も、履歴も、そしてこの記録を片付ける手段までもが同時に
		// 見えなくなる — 呼び出し側はこの一覧で設定画面全体を組み立てている。
		// 判別できない記録は、どちらの操作も提示しないまま並べる。Complete と
		// Rollback は、その記録に対しては引き続き同じ理由で拒否する。
		unresolved := errors.Is(reconcileErr, ErrRecoveryStateUnknown)
		switch {
		case unresolved:
		case reconcileErr != nil:
			return nil, reconcileErr
		case changed:
			journalPath := filepath.Join(m.journalDirectory(), record.ID+".json")
			if err := m.validateLoadedJournalRecord(record, filepath.Base(journalPath), m.journalDirectory()); err != nil {
				return nil, err
			}
			if err := m.writeRecord(journalPath, record); err != nil {
				return nil, err
			}
		}
		item := Pending{
			ID:          record.ID,
			Operation:   record.Operation,
			Status:      record.Status,
			StartedAt:   record.StartedAt,
			Committed:   record.Committed,
			CanComplete: !unresolved && record.Status == statusStaged && !record.Atomic,
			CanRollback: !unresolved,
		}
		for index, entry := range record.Entries {
			pendingEntry := PendingEntry{
				Path:      entry.Path,
				Target:    entry.Target,
				Action:    entry.action(),
				Committed: index < record.Committed,
				HasBackup: entry.Backup != "",
			}
			switch {
			case pendingEntry.Committed && pendingEntry.Action == actionRemove && entry.NoBackup:
				item.CanRollback = false
			case pendingEntry.Committed && pendingEntry.Action == actionWrite && entry.HadPrevious && entry.NoBackup:
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
	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return err
	}
	// Atomic records pair persisted documents with process-local state (the
	// opened password vault). Completing only the disk half after the original
	// callback failed would leave that state stale. Their safe recovery action
	// is rollback; a fresh request can then publish disk and memory together.
	if record.Atomic || record.Status != statusStaged {
		return ErrCannotComplete
	}
	for index := record.Committed; index < len(record.Entries); index++ {
		if record.Entries[index].action() != actionWrite {
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
	record, journalPath, err := m.loadPending(identifier)
	if err != nil {
		return err
	}
	return m.rollbackRecord(record, journalPath)
}

// rollbackRecord is also used by CommitAtomic. In that path the in-memory
// record can be ahead of the last durable journal update (for example when a
// target rename succeeded but SyncDir or the journal rewrite failed), so it is
// the only complete account of what this still-running process applied.
func (m *Manager) rollbackRecord(record *journalRecord, journalPath string) error {
	for index := 0; index < record.Committed; index++ {
		entry := record.Entries[index]
		// バックアップを残した削除は、置き換えと同じくらい可逆である。バイト列は
		// 世代ディレクトリにあり、モードはエントリにある。巻き戻せないのは、意図して
		// 何も残さなかったものだけである。
		if entry.action() == actionRemove && entry.NoBackup {
			return ErrIrreversibleRemoval
		}
		if entry.action() == actionWrite && entry.HadPrevious && entry.NoBackup {
			return ErrIrreversibleChange
		}
	}

	fileSystem := m.workspace.FileSystem()
	for index := record.Committed - 1; index >= 0; index-- {
		entry := record.Entries[index]
		if entry.action() == actionMove {
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
		if entry.action() == actionMakeDir {
			// もとからあったディレクトリは、このトランザクションが取り除いてよいもの
			// ではない。取り消すのはこれが作ったものだけであり、しかもまだ空である
			// 場合に限る — その後に何かが書き込まれているかもしれず、それを巻き戻しと
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
		if entry.action() == actionRemoveDir {
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
			contents, readErr := m.ReadBackup(entry.Backup)
			if readErr != nil {
				return readErr
			}
			writeErr := m.writeFile(entry.Path, contents, fs.FileMode(entry.Mode))
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
	for _, entry := range record.Entries {
		if entry.Temp == "" {
			continue
		}
		if err := fileSystem.Remove(entry.Temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
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

// reconcileRecord derives the applied prefix of an interrupted transaction from
// what its targets actually look like.
//
// Every target mutation happens before the journal rewrite that records it, so
// the durable Committed counter can sit behind the filesystem; a rollback that
// failed part way through leaves it ahead. Trusting the counter in either
// direction makes recovery report work it did not do — a Rollback that returns
// success while an applied write, move, or removal is still in place, and for a
// removal that intentionally kept no backup, that false success conceals
// irreversible data loss. Observation is the only account the writer and the
// reader both agree on, so it is recomputed before Pending, Complete, or
// Rollback uses the record.
//
// A non-atomic staging record is excluded. It has not reached commitStaged, so
// no target has been mutated; its only progress is directory creation, which
// the durable document deliberately never carries.
func (m *Manager) reconcileRecord(record *journalRecord) (bool, error) {
	if record.Status == statusCompleted || record.Status == statusRolledBack {
		return false, nil
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

	changed := record.Committed != committed
	record.Committed = committed
	for index := 0; index < committed; index++ {
		entry := &record.Entries[index]
		if entry.action() != actionWrite || entry.Temp == "" {
			continue
		}
		// 適用済みと数えたエントリの一時ファイルは、rename が使い切っているはず
		// である。証拠を持たない書き込みを内側に数えたときだけ実体が残るので、
		// 記録から名前を落とす前に消す。名前だけ落とせば、もう誰も片付けない
		// 断片が ~/.ssh に残る。
		if err := m.workspace.FileSystem().Remove(entry.Temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		entry.Temp = ""
		changed = true
	}
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
	// **ここを「適用済み」と読んではならない。** 直前が未適用のとき、ありもしない
	// 矛盾を作り出し、その記録は Pending も Complete も Rollback も永久に
	// 受け付けなくなる。
	evidenceNone
)

// entryApplied reports whether one entry's target mutation has already happened.
// A state that is neither the recorded before nor the recorded after is refused
// rather than guessed, because both recovery directions act on the answer.
func (m *Manager) entryEvidence(entry journalEntry) (entryEvidence, error) {
	switch entry.action() {
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
		if entry.HadPrevious && entry.Digest == entry.PreviousDigest {
			// 書いても中身が変わらない置き換え。application 層は metadata を
			// この形で毎回付けるので、これは例外ではなく日常の記録である。
			return evidenceNone, nil
		}
		digest, exists, err := m.targetDigest(entry.Path)
		if err != nil {
			return evidenceNone, err
		}
		switch {
		case !exists && !entry.HadPrevious:
			return evidenceUnapplied, nil
		case exists && digest == entry.Digest:
			return evidenceApplied, nil
		case exists && entry.HadPrevious && digest == entry.PreviousDigest:
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
	contents, err := m.workspace.FileSystem().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	digest := Digest(contents)
	zeroBytes(contents)
	return digest, true, nil
}

// readRecords は、ディレクトリ内のすべてのジャーナル文書を古い順に読み込む。
// 識別子は UTC のタイムスタンプで始まるので、辞書順は時系列順である。
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
		if validationErr := m.validateLoadedJournalRecord(record, name, directory); validationErr != nil {
			return nil, validationErr
		}
		records = append(records, record)
	}
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
		if record.Status != statusStaging && record.Status != statusStaged {
			return invalidJournal("unexpected pending status")
		}
	} else {
		if record.Status != statusCompleted && record.Status != statusRolledBack {
			return invalidJournal("unexpected history status")
		}
	}
	if record.Status == statusStaging || record.Status == statusStaged {
		if record.FinishedAt != nil {
			return invalidJournal("unfinished record has a finish time")
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
		if entry.Target != "" || entry.Mode&^uint32(FilePermission) != 0 || !digestValid {
			return invalidJournal("invalid write entry")
		}
		if record.Atomic && entry.NoBackup {
			return invalidJournal("atomic write cannot omit its backup")
		}
		if entry.HadPrevious != previousDigestValid {
			return invalidJournal("write previous digest mismatch")
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
		// エントリは何も引き起こさない — Complete は使う直前にステージ済みファイルを
		// すべて検証して拒否し、Pending は完了不可として報告する — ので、読み手は
		// 復旧そのものを立ち往生させる代わりにこれを受理する。
		if pending && record.Status == statusStaged && index < record.Committed && entry.Temp != "" && !record.Atomic {
			return invalidJournal("committed write retains a staged path")
		}
		if record.Status == statusCompleted && entry.Temp != "" {
			return invalidJournal("completed write retains a staged path")
		}
		if err := m.validateLoadedBackup(record, entry, pending); err != nil {
			return err
		}
	case actionMove:
		if entry.Target == "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || !entry.HadPrevious ||
			entry.Mode&^uint32(FilePermission) != 0 || !digestValid || entry.PreviousDigest != entry.Digest {
			return invalidJournal("invalid move entry")
		}
		if record.Atomic {
			return invalidJournal("atomic record contains a move")
		}
	case actionRemove:
		if entry.Target != "" || entry.Temp != "" || !entry.HadPrevious || entry.Mode&^uint32(FilePermission) != 0 ||
			!digestValid || entry.PreviousDigest != entry.Digest {
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
			entry.Digest != "" || entry.PreviousDigest != "" || entry.Mode != uint32(DirectoryPermission) {
			return invalidJournal("invalid mkdir entry")
		}
	case actionRemoveDir:
		if entry.Target != "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || !entry.HadPrevious ||
			entry.Digest != "" || entry.PreviousDigest != "" || entry.Mode&^uint32(0o777) != 0 {
			return invalidJournal("invalid rmdir entry")
		}
		if record.Atomic {
			return invalidJournal("atomic record contains rmdir")
		}
	case actionNote:
		if entry.Target != "" || entry.Temp != "" || entry.Backup != "" || entry.NoBackup || entry.HadPrevious ||
			entry.Digest != "" || entry.PreviousDigest != "" || entry.Mode != 0 {
			return invalidJournal("invalid note entry")
		}
	default:
		return invalidJournal("unknown entry action")
	}
	return nil
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
