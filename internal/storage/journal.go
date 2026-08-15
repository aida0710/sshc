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
	ErrAtomicStateUnknown = errors.New("an atomic transaction target no longer matches its old or staged state")
	ErrInvalidJournal     = errors.New("invalid transaction journal")
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
		if changed, reconcileErr := m.reconcileAtomicRecord(&record); reconcileErr != nil {
			return nil, reconcileErr
		} else if changed {
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
			CanComplete: record.Status == statusStaged && !record.Atomic,
			CanRollback: true,
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
			if err := m.writeFile(entry.Path, contents, fs.FileMode(entry.Mode)); err != nil {
				return err
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
	return Digest(contents) == entry.Digest
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
	if changed, err := m.reconcileAtomicRecord(&record); err != nil {
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

// reconcileAtomicRecord derives the applied prefix of a reversible atomic
// transaction from target digests. This is needed when the target rename made
// progress but both its journal update and the immediate rollback failed. A
// later recovery must not trust the stale Committed counter and declare the
// transaction rolled back while staged bytes remain on disk.
func (m *Manager) reconcileAtomicRecord(record *journalRecord) (bool, error) {
	if !record.Atomic || record.Status == statusCompleted || record.Status == statusRolledBack {
		return false, nil
	}
	applied := make([]bool, len(record.Entries))
	for index, entry := range record.Entries {
		switch entry.action() {
		case actionMakeDir:
			info, err := m.workspace.FileSystem().Lstat(entry.Path)
			switch {
			case err == nil && info.IsDir():
				applied[index] = true
			case errors.Is(err, fs.ErrNotExist):
				applied[index] = false
			case err != nil:
				return false, err
			default:
				return false, ErrAtomicStateUnknown
			}
		case actionWrite:
			contents, err := m.workspace.FileSystem().ReadFile(entry.Path)
			if errors.Is(err, fs.ErrNotExist) {
				if entry.HadPrevious {
					return false, ErrAtomicStateUnknown
				}
				applied[index] = false
				continue
			}
			if err != nil {
				return false, err
			}
			digest := Digest(contents)
			switch {
			case digest == entry.Digest:
				applied[index] = true
			case entry.HadPrevious && digest == entry.PreviousDigest:
				applied[index] = false
			default:
				return false, ErrAtomicStateUnknown
			}
		default:
			return false, ErrAtomicWriteOnly
		}
	}

	committed := 0
	gap := false
	for index, isApplied := range applied {
		if !isApplied {
			gap = true
			continue
		}
		if gap {
			// commit and rollback both move through the list in opposite order,
			// so an applied entry after a gap cannot be attributed safely.
			return false, ErrAtomicStateUnknown
		}
		committed = index + 1
	}
	changed := record.Committed != committed
	record.Committed = committed
	for index := 0; index < committed; index++ {
		if record.Entries[index].action() == actionWrite && record.Entries[index].Temp != "" {
			record.Entries[index].Temp = ""
			changed = true
		}
	}
	return changed, nil
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
		if pending && record.Status == statusStaged && index >= record.Committed && entry.Temp == "" && !record.Atomic {
			return invalidJournal("uncommitted write has no staged file")
		}
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
