package storage

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"sshc/internal/platform/nativepath"
)

// temporaryPrefix は、ステージ用の一時ファイルの名前の先頭。トランザクションの
// 一時ファイルは、この後ろに記録の ID と "-" が続く。
const temporaryPrefix = ".sshc-"

var (
	ErrNoChanges        = errors.New("transaction has no changes")
	ErrDuplicatePath    = errors.New("transaction contains the same path twice")
	ErrInvalidOperation = errors.New("transaction operation is required")
	// ErrDirectoryNotEmpty は、まだ何かを保持しているディレクトリの削除を拒否する。
	// 再帰的な削除にジャーナルの居場所はない。それを巻き戻すには、このトランザクション
	// が一度も読んでいない内容を復元しなければならなくなる。
	ErrDirectoryNotEmpty   = errors.New("directory is not empty")
	ErrIrreversibleChange  = errors.New("a committed change that kept no backup cannot be rolled back")
	ErrMoveTargetExists    = errors.New("move target already exists")
	ErrMissingSource       = errors.New("file to move or remove does not exist")
	ErrIrreversibleRemoval = errors.New("a committed removal that kept no backup cannot be rolled back")
	ErrAtomicWriteOnly     = errors.New("atomic commit accepts only reversible writes and directory creation")
)

// Manager は、ワークスペース内でジャーナル付きの原子的な複数ファイル書き込みを行う。
//
// Validate は、事前条件のあと、ジャーナルへの記録や書き込みの前に走る省略可能な
// 検査である。ストレージ層は意図的に設定の構文を何も知らない。アプリケーション層が
// バリデータを注入し、それが新しい内容を解析して Include グラフを再検査するので、
// 構文的に壊れたファイルがディスクへ届くことはない。Validate が nil なら、すべての
// リクエストを受け入れる。
type Manager struct {
	workspace *Workspace
	now       func() time.Time
	random    io.Reader
	Validate  func(Request) error
	// Seal と Unseal は、世代バックアップを暗号文にし、また戻す。
	//
	// これらを注入するのは、秘密がどこにあるかは secret パッケージの領分であり、
	// このパッケージはそれを尋ねるために import してはならないからだ。鍵 vault が
	// パスフレーズを探すために import しないのと同じ理屈である。Seal を持たない
	// マネージャは平文でバックアップを書く。マスターパスワードを持つ配線は、いずれも
	// 両方を設定する。
	Seal   func(plaintext []byte) ([]byte, error)
	Unseal func(sealed []byte) ([]byte, error)
	// AfterCommit reports a completed file mutation after the workspace lock has
	// been released. It is a notification only: the durable commit has already
	// succeeded and the callback must not attempt to change its result.
	AfterCommit func(operation string)
	// AfterRecovery は、保留記録を Complete か Rollback で片付けたあと、錠を手放して
	// から、その記録が触れたパスを知らせる。ディスクの内容をメモリに持つ側（ロックを
	// 解除した Vault など）が読み直すための通知であり、片付けの結果は変えられない。
	AfterRecovery func(paths []string)
}

// IsTemporaryName は、name がこのパッケージの一時ファイルの名前かを返す。
// クラッシュで残った一時ファイルを、設定や鍵、バックアップ、同期するファイルとして
// 読まないために、ディレクトリを走査する側が使う。大文字小文字を区別しない
// ファイルシステムから来た名前も同じに扱う。
func IsTemporaryName(name string) bool {
	return strings.HasPrefix(strings.ToLower(name), temporaryPrefix)
}

func NewManager(workspace *Workspace, now func() time.Time, random io.Reader) *Manager {
	return &Manager{workspace: workspace, now: now, random: random}
}

// Commit はすべての変更を検証し、意図をジャーナルへ記録し、新しい内容をすべて
// 永続的にステージし、そのうえでエントリをひとつずつ適用する。
//
// Commit は自動では巻き戻さない。失敗すると保留中のジャーナルが残るので、ユーザー
// は「完了させる」か「復元する」かを選べる。複数のファイルが関わるとき、それが
// 唯一の誠実な選択肢である。
func (m *Manager) Commit(request Request) (Result, error) {
	result, err := m.commit(request, commitMode{})
	if err == nil {
		m.notifyCommit(request.Operation)
	}
	return result, err
}

// CommitAtomic は Commit より厳しい失敗時規則でトランザクションを適用する。
// ステージ済みのファイル操作が失敗した場合、このプロセスで適用した操作をすべて
// ロールバックしてからエラーを返す。SSH 接続と暗号化されたパスワード割り当て、
// 鍵ファイルの移動とそのパスフレーズの割り当てのように、複数の永続化文書が 1 つの
// 論理値を表す場合に使用する。
//
// 受け付けるのは巻き戻せる操作だけである。移動・空のディレクトリの削除・控えを
// 残す削除は Rollback が元に戻せるが、控えを残さない書き込みと削除は戻せない。
func (m *Manager) CommitAtomic(request Request) (Result, error) {
	if err := validateAtomicRequest(request); err != nil {
		return Result{}, err
	}
	result, err := m.commit(request, commitMode{rollbackOnError: true})
	if err == nil {
		m.notifyCommit(request.Operation)
	}
	return result, err
}

// CommitAtomicDiscardBackups atomically replaces a set of already-encrypted
// documents. Previous bytes are journaled verbatim for rollback, rather than
// being sealed a second time, and are discarded at the transaction boundary.
// This is intentionally narrow: master-key rotation is its only caller.
func (m *Manager) CommitAtomicDiscardBackups(request Request) (Result, error) {
	if err := validateDiscardBackupsRequest(request); err != nil {
		return Result{}, err
	}
	result, err := m.commit(request, commitMode{rollbackOnError: true, discardBackups: true})
	if err == nil {
		m.notifyCommit(request.Operation)
	}
	return result, err
}

// CommitAtomicDiscardBackupsAndPublish performs the narrow rekey transaction
// and publishes its in-memory key generation before releasing the workspace
// mutation barrier. publish must be infallible: it runs only after the durable
// applied marker is the authoritative commit point.
//
// build runs while the workspace mutation barrier is held. The rekey lists the
// generation backups there, so a normal commit cannot add a backup sealed with
// the old key between that listing and the commit.
func (m *Manager) CommitAtomicDiscardBackupsAndPublish(build func() (Request, error), publish func()) (Result, error) {
	var request Request
	result, err := m.commitBuilt(func() (Request, error) {
		built, err := build()
		if err != nil {
			return Request{}, err
		}
		if err := validateDiscardBackupsRequest(built); err != nil {
			return Request{}, err
		}
		request = built
		return built, nil
	}, commitMode{rollbackOnError: true, discardBackups: true, publish: publish})
	if err == nil {
		m.notifyCommit(request.Operation)
	}
	return result, err
}

// validateAtomicRequest は、CommitAtomic が受け付ける要求を、失敗したときに巻き戻せる
// 操作に限る。書き込み・移動・ディレクトリの作成と空のディレクトリの削除・控えを残す
// 削除は Rollback が元に戻せるが、控えを残さない書き込みと削除は戻せない。
func validateAtomicRequest(request Request) error {
	if request.Operation == "" {
		return ErrInvalidOperation
	}
	for _, removal := range request.Removals {
		if !removal.Backup {
			return ErrIrreversibleRemoval
		}
	}
	if request.hasChangeWithoutBackup() {
		return ErrIrreversibleChange
	}
	return nil
}

// validateDiscardBackupsRequest は、バックアップを残さないトランザクションを、
// 暗号化済み文書の置き換えだけに限る。巻き戻しに使う前のバイト列は封じずに持ち、
// 境目で捨てるので、CommitAtomic が受け付ける移動・削除・ディレクトリの作成と削除も
// 断る。
func validateDiscardBackupsRequest(request Request) error {
	if request.Operation == "" {
		return ErrInvalidOperation
	}
	if len(request.Directories) > 0 || len(request.Moves) > 0 || len(request.Removals) > 0 ||
		len(request.RemoveDirectories) > 0 || request.hasChangeWithoutBackup() {
		return ErrAtomicWriteOnly
	}
	return nil
}

func (m *Manager) notifyCommit(operation string) {
	if m.AfterCommit != nil {
		m.AfterCommit(operation)
	}
}

func (m *Manager) notifyRecovery(paths []string) {
	if m.AfterRecovery != nil && len(paths) > 0 {
		m.AfterRecovery(paths)
	}
}

// WithSnapshot holds the same process and OS mutation barrier as Commit while
// WithSnapshot runs snapshot, which reads a coherent workspace generation.
// Callers that also coordinate secrets must acquire their secret mutation
// barrier before entering here, matching the established secret-writer ->
// workspace lock order.
func (m *Manager) WithSnapshot(snapshot func() error) error {
	if snapshot == nil {
		return nil
	}
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return err
	}
	defer unlock()
	if err := m.ensureNoPendingTransaction(); err != nil {
		return err
	}
	return snapshot()
}

func (m *Manager) ensureNoPendingTransaction() error {
	records, err := m.readRecords(m.journalDirectory(), refuseOtherJournalVersions)
	if err != nil {
		return err
	}
	if len(records) != 0 {
		return ErrPendingTransaction
	}
	return nil
}

// journalPlan は、これから記録するエントリと、それに紐づく内容をひとつの値にする。
//
// 3 つのスライスは添字で対応する。journalPlan に追加処理を集約し、異なる添字の
// エントリ、ステージ済みデータ、以前の内容を関連付けないようにする。
//
// staged は、これから書く内容（ファイルの置き換えだけが持つ）。previous は、置き換え
// られる前の内容（バックアップを取る操作だけが持つ）。どちらも無い操作では nil である。
type journalPlan struct {
	entries  []journalEntry
	staged   [][]byte
	previous [][]byte
}

func newJournalPlan(capacity int) *journalPlan {
	return &journalPlan{
		entries:  make([]journalEntry, 0, capacity),
		staged:   make([][]byte, 0, capacity),
		previous: make([][]byte, 0, capacity),
	}
}

// add は、エントリひとつと、それに紐づく内容を同時に足す。
func (p *journalPlan) add(entry journalEntry, staged, previous []byte) {
	p.entries = append(p.entries, entry)
	p.staged = append(p.staged, staged)
	p.previous = append(p.previous, previous)
}

// commitMode は、commit が失敗したときの扱いと、以前の内容の後片付けを選ぶ。
type commitMode struct {
	// rollbackOnError は、適用の途中で失敗したら、このプロセスが適用した分を巻き戻して
	// から返す（CommitAtomic）。偽なら保留の記録を残し、利用者に完了か復元を選ばせる。
	rollbackOnError bool
	// discardBackups は、以前の内容を中断したときの巻き戻しのためだけに残し、commit
	// point を越えたら消す（マスターパスワード変更）。
	discardBackups bool
	// publish は、commit point を越えたあと、workspace のロックを放す前に呼ぶ。失敗しては
	// ならない。nil なら何もしない。
	publish func()
}

func (m *Manager) commit(request Request, mode commitMode) (Result, error) {
	// 形の誤りはロックを待たずに返す。
	if err := validateRequestShape(request); err != nil {
		return Result{}, err
	}
	return m.commitBuilt(func() (Request, error) { return request, nil }, mode)
}

func validateRequestShape(request Request) error {
	if request.Operation == "" {
		return ErrInvalidOperation
	}
	if len(request.Changes)+len(request.FinalChanges)+len(request.Moves)+len(request.Removals)+
		len(request.Directories)+len(request.RemoveDirectories) == 0 {
		return ErrNoChanges
	}
	return nil
}

// commitBuilt は、workspace の変更のロックを取ってから build で request を組み立て、
// そのまま commit する。
func (m *Manager) commitBuilt(build func() (Request, error), mode commitMode) (Result, error) {
	unlock, err := m.workspace.lockMutation()
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	if err := m.ensureNoPendingTransaction(); err != nil {
		return Result{}, err
	}
	request, err := build()
	if err != nil {
		return Result{}, err
	}
	if err := validateRequestShape(request); err != nil {
		return Result{}, err
	}
	// 計画を組むのは planCommit である。ここから下はもう組み立てない。
	// 記録し、ステージし、置き換えるだけである。
	plan, written, err := m.planCommit(request)
	if err != nil {
		return Result{}, err
	}
	if m.Validate != nil {
		if err := m.Validate(request); err != nil {
			return Result{}, err
		}
	}
	run := &commitRun{manager: m, mode: mode, plan: plan}
	if err := run.begin(request.Operation, written); err != nil {
		return Result{}, err
	}
	return run.apply()
}

// commitStaged は、ステージ済みのエントリを記録の順に適用し、メモリ上の進捗
// （Committed と、rename が使い切った Temp）を進める。
//
// 進捗をエントリごとに journal へ書き直すことはしない。記録全体を毎回書き直すと
// 時間がエントリ数の 2 乗で伸び、バックアップを数千件抱えるマスターパスワード変更では
// workspace のロックを分単位で持ち続けるからである。書かなくても復旧は困らない。
// Pending、Complete、Rollback は記録を使う前に reconcileRecord で対象の状態から
// 進捗を数え直し、永続化した Committed を使わない。失敗したときは、呼び出し側の fail が
// このプロセスの知っている進捗を書き残す。
func (m *Manager) commitStaged(record *journalRecord) error {
	fileSystem := m.workspace.FileSystem()
	for index := record.Committed; index < len(record.Entries); index++ {
		entry := record.Entries[index]
		switch entry.Action {
		case actionMove:
			if err := m.moveFile(entry.Path, entry.Target); err != nil {
				return err
			}
			record.Committed = index + 1
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
			if err := fileSystem.SyncDir(filepath.Dir(entry.Target)); err != nil {
				return err
			}
		case actionRemove:
			if err := fileSystem.Remove(entry.Path); err != nil {
				return err
			}
			record.Committed = index + 1
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
		case actionMakeDir:
			if err := m.workspace.EnsureDirectory(entry.Path); err != nil {
				return err
			}
			record.Committed = index + 1
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
		case actionRemoveDir:
			if err := fileSystem.Remove(entry.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			record.Committed = index + 1
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
		default:
			if err := fileSystem.Rename(entry.Temp, entry.Path); err != nil {
				return err
			}
			// rename がステージ済みファイルを消費する。進捗の数え上げより先にそれを
			// 消しておくと、記録は途中のどの時点でも読み手の受理する形のままなので、
			// 失敗経路はそれをそのまま永続化できる。
			record.Entries[index].Temp = ""
			record.Committed = index + 1
			if err := fileSystem.SyncDir(filepath.Dir(entry.Path)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) moveFile(oldPath, newPath string) error {
	fileSystem := m.workspace.FileSystem()
	if m.isPrivateStatePath(oldPath) || m.isPrivateStatePath(newPath) {
		return fileSystem.MovePrivate(oldPath, newPath)
	}
	return fileSystem.Rename(oldPath, newPath)
}

func (m *Manager) isPrivateStatePath(path string) bool {
	return privateStateContains(m.workspace.StateDir(), path)
}

func privateStateContains(stateDirectory, path string) bool {
	return nativepath.Contains(stateDirectory, path)
}

// sourceState は、これから移動または削除されるファイルをハッシュし、呼び出し側の
// 事前条件を検査する。
//
// ダイジェストができた時点でバイト列をゼロで埋める。移動も削除も二度とそれを必要と
// せず、そのファイルは秘密鍵かもしれないからだ。返される ConflictError が意図的に
// Current の内容を運ばないのも同じ理由である。鍵素材の三方向差分は無用でもあり
// 危険でもある。
func (m *Manager) sourceState(path string, precondition Precondition) (string, fs.FileMode, error) {
	contents, mode, exists, err := m.currentState(path)
	if err != nil {
		return "", 0, err
	}
	if !exists {
		return "", 0, ErrMissingSource
	}
	digest := Digest(contents)
	clear(contents)

	expected := ""
	if precondition.Exists {
		expected = precondition.Digest
	}
	if digest != expected {
		return "", 0, &ConflictError{Path: path, Expected: expected, Actual: digest}
	}
	if precondition.Mode != 0 && !m.ownerModesMatch(mode, precondition.Mode) {
		return "", 0, &ConflictError{Path: path, Expected: expected, Actual: digest}
	}
	return digest, mode, nil
}

func (m *Manager) finish(record *journalRecord, journalPath, status string) error {
	fileSystem := m.workspace.FileSystem()
	// ステージ済みファイルを手放すのは、錠を持ったここと、staging のまま失敗した
	// commit だけである。完了なら rename が使い切っており、巻き戻しなら捨てる。
	// どちらでも、記録が履歴になる前に名前と対象の両方が消える。復旧がこれを読む
	// だけの経路でやると、走っている最中のトランザクションの一時ファイルを消せて
	// しまう。
	if err := m.removeStagedTemps(record); err != nil {
		return err
	}
	if err := m.removeLeftoverTemps(record); err != nil {
		return err
	}
	if record.DiscardBackups {
		if err := m.discardRollbackBackups(record); err != nil {
			return err
		}
	}
	finished := m.now().UTC()
	record.FinishedAt = &finished
	record.Status = status
	historyPath := filepath.Join(m.workspace.StateDir(), historyDirectoryName, record.ID+".json")
	if err := m.writeRecord(historyPath, *record); err != nil {
		return err
	}
	if err := fileSystem.Remove(journalPath); err != nil {
		return err
	}
	if err := fileSystem.SyncDir(filepath.Dir(journalPath)); err != nil {
		return err
	}
	// 保持の上限を超えた記録と控えは、変更が完了したこの時点で消す。消せなくても
	// この変更の結果は変えない。次の完了か sshcエンジンの起動で、もう一度消す。
	_ = m.pruneHistory(record.ID)
	return nil
}

// removeStagedTemps は、記録が名前を持つステージ済みファイルを消し、名前を手放す。
func (m *Manager) removeStagedTemps(record *journalRecord) error {
	fileSystem := m.workspace.FileSystem()
	for index := range record.Entries {
		temp := record.Entries[index].Temp
		if temp == "" {
			continue
		}
		if err := fileSystem.Remove(temp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		record.Entries[index].Temp = ""
	}
	return nil
}

// removeLeftoverTemps は、この記録の ID で始まる一時ファイルのうち、記録に名前が
// 載らなかったものを消す。ステージの途中でプロセスが落ちると、staging の記録は
// 一時ファイルの名前を持たないまま残る。ID はトランザクションごとに一意なので、
// 走っている別のトランザクションの一時ファイルは消さない。
func (m *Manager) removeLeftoverTemps(record *journalRecord) error {
	fileSystem := m.workspace.FileSystem()
	prefix := temporaryPrefix + record.ID + "-"
	scanned := map[string]bool{}
	for _, entry := range record.Entries {
		directory := filepath.Dir(entry.Path)
		if entry.Action != actionWrite || scanned[directory] {
			continue
		}
		scanned[directory] = true
		children, err := fileSystem.ReadDir(directory)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, child := range children {
			if !strings.HasPrefix(child.Name(), prefix) {
				continue
			}
			if err := fileSystem.Remove(filepath.Join(directory, child.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) finishApplied(record *journalRecord, journalPath string) error {
	if record.Status != statusApplied || !record.DiscardBackups || record.Committed != len(record.Entries) {
		return ErrCannotComplete
	}
	return m.finish(record, journalPath, statusCompleted)
}

// currentState は、置き換えられるファイルを読む。返されるモードはownerの権限だけを
// 保ち、group/otherの権限は落とす。これにより既存の0700は維持しつつ、0644のような
// 緩い権限を次の書き込みで0600へ締める。
func (m *Manager) currentState(path string) (contents []byte, mode fs.FileMode, exists bool, err error) {
	info, err := m.workspace.FileSystem().Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, FilePermission, false, nil
	}
	if err != nil {
		return nil, 0, false, err
	}
	contents, err = m.workspace.ReadTransactionFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	return contents, info.Mode().Perm() & 0o700, true, nil
}

// ownerModesMatch は、owner の権限をこのワークスペースの FileSystem が読み返せる範囲
// だけで比べる。Windows では 0700 で書いたファイルが 0600 と読めるので、そのまま
// 比べると自分の書いた姿を他人の変更と取り違える。
func (m *Manager) ownerModesMatch(left, right fs.FileMode) bool {
	fileSystem := m.workspace.FileSystem()
	return observableOwnerMode(fileSystem, left) == observableOwnerMode(fileSystem, right)
}

func (m *Manager) writeFile(path string, contents []byte, permission fs.FileMode) error {
	return WriteAtomicFile(m.workspace.FileSystem(), path, temporaryPrefix, permission, contents)
}
