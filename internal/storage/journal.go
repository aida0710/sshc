package storage

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sshc/internal/platform/nativepath"
)

const (
	journalVersion       = 2
	journalDirectoryName = "journal"
	historyDirectoryName = "history"

	statusStaging    = "staging"
	statusStaged     = "staged"
	statusApplied    = "applied"
	statusCompleted  = "completed"
	statusRolledBack = "rolled_back"
)

const (
	actionWrite     = "write"
	actionMove      = "move"
	actionRemove    = "remove"
	actionMakeDir   = "mkdir"
	actionRemoveDir = "rmdir"
	actionNote      = "note"
)

var ErrInvalidJournal = errors.New("invalid transaction journal")

// journalVersionRelease は、journalVersion の記録を書き始めた sshc の版。
// JournalVersionError に載せ、利用者が手元の版と照らせるようにする。
const journalVersionRelease = "v0.24.0"

// JournalVersionError は、中断した変更の記録が journalVersion と違う版で書かれて
// いて、この版では完了も巻き戻しもできないことを報告する。起動時の自動ロック解除も
// この記録で止まるので、利用者が手で扱えるように記録の ID とファイルのパスを持つ。
// errors.Is(err, ErrInvalidJournal) は true になる。
type JournalVersionError struct {
	ID      string
	Path    string
	Version int
}

func (e *JournalVersionError) Error() string {
	recordedBy := fmt.Sprintf("an sshc release before %s", journalVersionRelease)
	if e.Version > journalVersion {
		recordedBy = "a newer sshc release"
	}
	return fmt.Sprintf("%s: interrupted change %s (%s) was recorded by %s (journal version %d); this release cannot complete or roll it back",
		ErrInvalidJournal, e.ID, e.Path, recordedBy, e.Version)
}

func (e *JournalVersionError) Unwrap() error { return ErrInvalidJournal }

var journalIdentifierPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{3}-[0-9a-f]{8}$`)

// journalIdentifierTimeLayout は、記録の ID の先頭にある、変更を始めた時刻（UTC）の書式。
// ID の辞書順はこの時刻の順になる。
const journalIdentifierTimeLayout = "20060102T150405.000"

type journalEntry struct {
	Action      string `json:"action,omitempty"`
	Path        string `json:"path"`
	Target      string `json:"target,omitempty"`
	Temp        string `json:"temp,omitempty"`
	Backup      string `json:"backup,omitempty"`
	NoBackup    bool   `json:"noBackup,omitempty"`
	HadPrevious bool   `json:"hadPrevious"`
	// Mode is the state after a write. PreviousMode is the state before it.
	// Other actions keep their single existing mode in Mode.
	Mode           uint32 `json:"mode"`
	PreviousMode   uint32 `json:"previousMode,omitempty"`
	Digest         string `json:"digest"`
	PreviousDigest string `json:"previousDigest,omitempty"`
}

// noOpWrite は、書いても中身が変わらない置き換えを言う。
//
// application 層は metadata の書き込みを毎回、変わっていなくても最後に足すので、
// これは例外ではなく日常の記録である。巻き戻せない変更ではない。戻したあとの
// 対象は同じバイト列であり、控えを残さなかったとしても失うものが無い。
func (e journalEntry) noOpWrite() bool {
	return e.sameContentsWrite() && e.Mode == e.PreviousMode
}

func (e journalEntry) sameContentsWrite() bool {
	return e.Action == actionWrite && e.HadPrevious && e.Digest == e.PreviousDigest
}

// checkReversible は、適用済みのこのエントリを巻き戻せるかを確かめる。
//
// バックアップを残した削除は、置き換えと同じくらい可逆である。バイト列は
// 世代ディレクトリにあり、モードはエントリにある。巻き戻せないのは、意図して
// 何も残さなかったものだけである。
func (e journalEntry) checkReversible() error {
	if e.Action == actionRemove && e.NoBackup {
		return ErrIrreversibleRemoval
	}
	if e.Action == actionWrite && e.HadPrevious && e.NoBackup && !e.sameContentsWrite() {
		return ErrIrreversibleChange
	}
	return nil
}

type journalRecord struct {
	ID         string     `json:"id"`
	Version    int        `json:"version"`
	Operation  string     `json:"operation"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Committed  int        `json:"committed"`
	Atomic     bool       `json:"atomic,omitempty"`
	// DiscardBackups marks an atomic replacement whose previous bytes exist only
	// to recover an interrupted commit. They bypass the normal backup sealer and
	// are removed once either the old or the new generation is authoritative.
	DiscardBackups bool           `json:"discardBackups,omitempty"`
	Entries        []journalEntry `json:"entries"`
}

// touchedPaths は、この記録が書く・消す・動かす対象のパスを返す。
func (record *journalRecord) touchedPaths() []string {
	paths := make([]string, 0, len(record.Entries))
	for _, entry := range record.Entries {
		paths = append(paths, entry.Path)
		if entry.Target != "" {
			paths = append(paths, entry.Target)
		}
	}
	return paths
}

func (m *Manager) journalDirectory() string {
	return filepath.Join(m.workspace.StateDir(), journalDirectoryName)
}

func (m *Manager) historyDirectory() string {
	return filepath.Join(m.workspace.StateDir(), historyDirectoryName)
}

func (m *Manager) writeRecord(path string, record journalRecord) error {
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if len(contents) > maxRecordBytes {
		return ErrTransactionTooLarge
	}
	return m.writeFile(path, contents, FilePermission)
}

func (m *Manager) newIdentifier() (string, error) {
	suffix := make([]byte, 4)
	if _, err := io.ReadFull(m.random, suffix); err != nil {
		return "", err
	}
	return m.now().UTC().Format(journalIdentifierTimeLayout) + "-" + hex.EncodeToString(suffix), nil
}

// otherJournalVersionPolicy は、journalVersion と違う版の記録を読んだときの扱い。
// 旧版の記録を今の版へ直して読むことはしない。
type otherJournalVersionPolicy int

const (
	// refuseOtherJournalVersions は、中断した変更の記録に使う。読めない記録を飛ばすと、
	// 中断した変更を片付けないまま次の変更を書いてしまうので、JournalVersionError で断る。
	refuseOtherJournalVersions otherJournalVersionPolicy = iota
	// skipOtherJournalVersions は、完了した変更の履歴に使う。履歴は表示と復元の候補に
	// 使うだけなので、版の違う記録は一覧から外し、ほかの記録を見せ続ける。
	skipOtherJournalVersions
)

// readRecords は、ディレクトリ内のすべてのジャーナル文書を古い順に読み込む。
//
// 識別子はミリ秒までの UTC タイムスタンプで始まり、そのあとに乱数が続く。同じ
// ミリ秒に落ちた 2 件の間では、辞書順は時系列順ではない。接頭辞が一致するので、
// 順序を決めるのは乱数になる。そこで並べ替えは記録が保持しているナノ秒精度の
// StartedAt で行い、識別子は同時刻の決定的なタイブレークにだけ使う。
//
// 読み込み自体は名前順で行う。ここが時系列である必要はないが、壊れた文書に当たった
// ときに返るエラーが実行ごとに変わらないほうが調べやすい。
func (m *Manager) readRecords(directory string, otherVersions otherJournalVersionPolicy) ([]journalRecord, error) {
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
		contents, readErr := ReadFileLimited(m.workspace.FileSystem(), filepath.Join(directory, name), maxRecordBytes)
		if readErr != nil {
			return nil, readErr
		}
		var record journalRecord
		if unmarshalErr := json.Unmarshal(contents, &record); unmarshalErr != nil {
			clear(contents)
			return nil, unmarshalErr
		}
		clear(contents)
		if record.Version != journalVersion && otherVersions == skipOtherJournalVersions {
			continue
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

func (m *Manager) validateLoadedJournalRecord(record journalRecord, name, directory string) error {
	identifier, err := journalIdentifierFromName(name)
	if err != nil {
		return err
	}
	if record.ID != identifier {
		return invalidJournal("identity mismatch")
	}
	if record.Version != journalVersion {
		return &JournalVersionError{ID: identifier, Path: filepath.Join(directory, name), Version: record.Version}
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
	claimed := newClaimedPaths(len(record.Entries) * 2)
	for index, entry := range record.Entries {
		if err := m.validateLoadedJournalEntry(record, entry, index, pending); err != nil {
			return err
		}
		if !claimed.claim(entry.Path) {
			return invalidJournal("duplicate entry path")
		}
		if entry.Target != "" && !claimed.claim(entry.Target) {
			return invalidJournal("duplicate entry target")
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
	relative, ok := nativepath.RelativeBelow(m.workspace.Root(), entry.Path)
	if !ok {
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
