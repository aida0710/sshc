package sftp

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

const AbsentRevision = "absent"

var transferIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
var sourceFingerprintPattern = regexp.MustCompile(`^tree-sha256:[0-9a-f]{64}$`)
var uploadPartNamePattern = regexp.MustCompile(`^\..+\.sshc-upload-[A-Za-z0-9_-]{8,128}\.part$`)
var editorTemporaryNamePattern = regexp.MustCompile(`^\..+\.sshc-[0-9a-f]{24}\.tmp$`)

// TransferManager は、engine が抱える転送の全体である。3 つの関心ごとに 1 つずつ
// lock を持ち、field はその lock の下に並べる。
//
//   - データプレーン（mutex）: 対象ごとの operation lock、開いた remote、
//     prepared download の spool。upload_plane.go／download_plane.go／spool.go。
//     同じ再開用の part file への操作は operation lock で直列にし、別のファイルへの
//     操作は並行に進める。
//   - job 台帳（jobsMutex）: queue の記録・順序・設定・永続化。jobs.go（台帳の
//     初期化・一覧・写し）、jobs_types.go、jobs_admission.go（受け入れと追い出し）、
//     jobs_state.go（状態遷移）、jobs_order.go、jobs_removal.go、jobs_expiry.go
//     （滞留と期限切れ）、jobs_settings.go、transfer_settings.go、queue_store.go。
//   - remote worker（remoteJobsMutex）: engine 内で走る copy／move／delete／
//     put／get。remote_jobs.go。
//
// `*Owned` の API は job に属する転送で、データプレーンを進めながら台帳の
// record を更新する。両方の lock を取る唯一の層である。
type TransferManager struct {
	Service *Service

	// データプレーン。mutex が守る。
	mutex   sync.Mutex
	locks   map[string]*transferLock
	remotes map[string]Remote
	// idleRemotes marks the connections in remotes that no request holds:
	// the ones a sequential upload keeps for its next chunk.
	idleRemotes map[string]*idleTransferRemote
	// after schedules the discard of an idle connection. Tests replace it to
	// fire the discard when they choose. It runs under mutex, so it must not
	// call the discard before returning.
	after     func(time.Duration, func()) *time.Timer
	downloads map[string]preparedDownloadCache
	spool     *downloadSpool
	closed    bool
	closeOnce sync.Once
	closeErr  error

	// job 台帳。jobsMutex が守る。
	jobsMutex sync.Mutex
	jobs      map[string]*transferJobRecord
	jobOrder  []string
	// clearCompletedAfter が 0 なら、完了項目は手動でだけ消える。
	clearCompletedAfter time.Duration
	// processingStopped の間は start を通さない。実行中のものは走り切る。
	processingStopped    bool
	activeJobs           int
	maxConcurrent        int
	largeFileThreshold   int64
	largeFileParallelism int
	largeFileChunkBytes  int64
	now                  func() time.Time
	dataPlane            map[string]int
	queuePath            string
	lastQueuePersist     time.Time
	queuePersistError    error
	// slotReleased is closed when a slot may have opened; see slotWait.
	slotReleased chan struct{}

	// remote worker。remoteJobsMutex が守る。
	remoteJobsMutex sync.Mutex
	remoteRuns      map[string]*remoteRun
	remoteWorkers   sync.WaitGroup

	// 転送キューの設定の更新。settingsMutex が一つずつ通す。
	settingsMutex sync.Mutex
	// saveSettings は、engine に適用する前に設定を残す。nil なら設定は
	// engine の process が生きているあいだだけ効く。
	saveSettings func(TransferSettings) error
}

// transferRemoteIdleTimeout is how long a sequential upload keeps its
// connection for a next chunk that does not come. A client sends the next
// chunk as soon as the previous one returns, and the stale sweep counts a job
// silent this long as gone. That sweep only runs when a request reaches the
// queue, and after every sshc window closed none may come.
const transferRemoteIdleTimeout = staleRunningTransferAfter

// idleTransferRemote is one period during which a connection kept in remotes
// waits for its next request. Each period has its own mark, so the timer of a
// period that a request already ended finds a different mark and does nothing.
type idleTransferRemote struct {
	remote Remote
	// timer discards the connection when the period runs out. Ending the
	// period stops it, so a sequential upload sending a chunk per request
	// keeps one timer alive rather than one per chunk.
	timer *time.Timer
}

// endIdlePeriodLocked removes the idle mark of key and stops its timer. The
// caller holds m.mutex.
func (m *TransferManager) endIdlePeriodLocked(key string) {
	idle := m.idleRemotes[key]
	if idle == nil {
		return
	}
	delete(m.idleRemotes, key)
	if idle.timer != nil {
		idle.timer.Stop()
	}
}

type transferLock struct {
	mutex sync.RWMutex
	refs  int
}

// NewTransferManager prepares downloads under downloadSpoolRoot, a directory
// that only this user can write and that every sshc process of the user
// shares for the quota. The caller chooses it: the engine passes the user's
// cache, Android the app's cache and tests their own temporary directory, so
// this package never guesses from HOME. The directory is created when missing.
// With an empty root, downloads fail with ErrSpoolUnavailable.
func NewTransferManager(service *Service, downloadSpoolRoot string) *TransferManager {
	manager := &TransferManager{
		Service: service, locks: make(map[string]*transferLock), remotes: make(map[string]Remote),
		idleRemotes: make(map[string]*idleTransferRemote), after: time.AfterFunc,
		downloads: make(map[string]preparedDownloadCache), spool: newDownloadSpool(downloadSpoolRoot),
		remoteRuns: make(map[string]*remoteRun),
	}
	manager.ConfigureJobs(DefaultTransferConcurrency, time.Now)
	return manager
}

// DownloadSpoolError reports why the spool root given to NewTransferManager
// cannot hold downloads, or nil when it can. The engine logs it at start,
// because each download only reports sftp_spool_unavailable.
func (m *TransferManager) DownloadSpoolError() error {
	return m.spool.rootErr
}

// Close releases resources intentionally retained between transfer requests.
// The HTTP server calls it only after its admission barrier has drained every
// request, so no data-plane operation can still be using these handles.
func (m *TransferManager) Close() error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.mutex.Lock()
		m.closed = true
		remotes := make([]Remote, 0, len(m.remotes))
		for key, remote := range m.remotes {
			delete(m.remotes, key)
			remotes = append(remotes, remote)
		}
		for key := range m.idleRemotes {
			m.endIdlePeriodLocked(key)
		}
		downloads := make([]*PreparedDownload, 0, len(m.downloads))
		for id, cached := range m.downloads {
			delete(m.downloads, id)
			downloads = append(downloads, cached.download)
		}
		m.mutex.Unlock()
		m.remoteJobsMutex.Lock()
		for _, run := range m.remoteRuns {
			run.cancel()
		}
		m.remoteJobsMutex.Unlock()
		// A remote-to-remote worker owns request-scoped SFTP connections which
		// are not present in m.remotes. Wait for cancellation to close those
		// connections and for the goroutine to leave before server shutdown
		// reports completion.
		m.remoteWorkers.Wait()
		var joined []error
		for _, remote := range remotes {
			if err := remote.Close(); err != nil {
				joined = append(joined, err)
			}
		}
		for _, download := range downloads {
			if err := download.Close(); err != nil {
				joined = append(joined, err)
			}
		}
		if err := m.spool.close(); err != nil {
			joined = append(joined, err)
		}
		m.closeErr = errors.Join(joined...)
	})
	return m.closeErr
}

func (m *TransferManager) isClosed() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.closed
}

// LockOperation serializes every public mutation which can affect the given
// remote paths. Sorting canonical paths gives Rename a stable two-lock order
// and prevents two opposite renames from deadlocking.
func (m *TransferManager) LockOperation(alias string, remotePaths ...string) (func(), error) {
	if m == nil || m.Service == nil || m.isClosed() {
		return nil, ErrUnavailable
	}
	if err := validateAlias(alias); err != nil {
		return nil, err
	}
	unique := make(map[string]struct{}, len(remotePaths))
	paths := make([]string, 0, len(remotePaths))
	for _, remotePath := range remotePaths {
		cleaned, err := cleanPublicPath(remotePath, false)
		if err != nil {
			return nil, err
		}
		if _, exists := unique[cleaned]; exists {
			continue
		}
		unique[cleaned] = struct{}{}
		paths = append(paths, cleaned)
	}
	if len(paths) == 0 {
		return nil, ErrInvalidPath
	}
	sort.Strings(paths)
	unlocks := make([]func(), 0, len(paths))
	for _, remotePath := range paths {
		unlocks = append(unlocks, m.lock(alias, remotePath))
	}
	if m.isClosed() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
		return nil, ErrUnavailable
	}
	return func() {
		for index := len(unlocks) - 1; index >= 0; index-- {
			unlocks[index]()
		}
	}, nil
}

func (m *TransferManager) lock(alias, target string) func() {
	return m.acquireLock(alias, target, false)
}

func (m *TransferManager) readLock(alias, target string) func() {
	return m.acquireLock(alias, target, true)
}

func (m *TransferManager) acquireLock(alias, target string, shared bool) func() {
	key := alias + "\x00" + target
	m.mutex.Lock()
	entry := m.locks[key]
	if entry == nil {
		entry = &transferLock{}
		m.locks[key] = entry
	}
	entry.refs++
	m.mutex.Unlock()
	if shared {
		entry.mutex.RLock()
	} else {
		entry.mutex.Lock()
	}
	return func() {
		if shared {
			entry.mutex.RUnlock()
		} else {
			entry.mutex.Unlock()
		}
		m.mutex.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(m.locks, key)
		}
		m.mutex.Unlock()
	}
}

func transferRemoteKey(alias, id, target string) string {
	return alias + "\x00" + id + "\x00" + target
}

// jobOwnerLockKey is the operation lock that serializes everything owning a
// job's upload state: publishing an upload and pausing, failing or removing
// the job. Every caller must use this key; a differently spelled one would be
// a separate mutex and silently drop that exclusion.
func jobOwnerLockKey(id string) string {
	return "\x00job-owner:" + id
}

func (m *TransferManager) transferRemote(ctx context.Context, alias, id, target string) (Remote, error) {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return nil, ErrUnavailable
	}
	remote := m.remotes[key]
	m.endIdlePeriodLocked(key)
	m.mutex.Unlock()
	if remote != nil {
		return remote, nil
	}
	remote, err := m.Service.open(ctx, alias)
	if err != nil {
		return nil, err
	}
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		_ = remote.Close()
		return nil, ErrUnavailable
	}
	if existing := m.remotes[key]; existing != nil {
		m.mutex.Unlock()
		_ = remote.Close()
		return existing, nil
	}
	m.remotes[key] = remote
	m.mutex.Unlock()
	return remote, nil
}

// keepRemoteIdle leaves a connection in remotes for the next chunk of the same
// sequential upload, and discards it if no request takes it within
// transferRemoteIdleTimeout.
func (m *TransferManager) keepRemoteIdle(alias, id, target string, remote Remote) {
	key := transferRemoteKey(alias, id, target)
	idle := &idleTransferRemote{remote: remote}
	m.mutex.Lock()
	if m.closed || m.remotes[key] != remote {
		// Close or a cancelled request has already taken the connection.
		m.mutex.Unlock()
		return
	}
	m.endIdlePeriodLocked(key)
	m.idleRemotes[key] = idle
	// after only schedules the discard, so it may run under the lock that
	// makes the timer part of the mark before any request can end the period.
	idle.timer = m.after(transferRemoteIdleTimeout, func() { m.discardIdleRemote(key, idle) })
	m.mutex.Unlock()
}

// discardIdleRemote closes a connection that stayed idle for the whole
// period idle marks. A request that took the connection in the meantime
// ended that period, and the connection is then left alone.
func (m *TransferManager) discardIdleRemote(key string, idle *idleTransferRemote) {
	m.mutex.Lock()
	if m.idleRemotes[key] != idle || m.remotes[key] != idle.remote {
		m.mutex.Unlock()
		return
	}
	delete(m.idleRemotes, key)
	delete(m.remotes, key)
	m.mutex.Unlock()
	discardRemote(idle.remote)
}

// watchRemoteCancellation closes and detaches a retained upload transport only
// when the current request is cancelled. Normal request completion stops the
// watcher and preserves the connection for the next chunk.
func (m *TransferManager) watchRemoteCancellation(ctx context.Context, alias, id, target string, remote Remote) func() {
	stop := context.AfterFunc(ctx, func() { m.releaseRemoteIf(alias, id, target, remote) })
	return func() { stop() }
}

// SaveText serializes editor publication with resumable upload publication
// for the same alias and target in this engine generation.
func (m *TransferManager) SaveText(ctx context.Context, alias, remotePath, contents, expectedRevision string) (TextFile, error) {
	if m == nil || m.Service == nil || m.isClosed() {
		return TextFile{}, ErrUnavailable
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	unlock := m.lock(alias, cleaned)
	defer unlock()
	return m.Service.SaveText(ctx, alias, cleaned, contents, expectedRevision)
}

func (m *TransferManager) releaseRemote(alias, id, target string) {
	remote := m.detachRemote(alias, id, target)
	if remote != nil {
		_ = remote.Close()
	}
}

func (m *TransferManager) releaseRemoteIf(alias, id, target string, expected Remote) {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	remote := m.remotes[key]
	if remote == expected {
		delete(m.remotes, key)
		m.endIdlePeriodLocked(key)
	} else {
		remote = nil
	}
	m.mutex.Unlock()
	if remote != nil {
		discardRemote(remote)
	}
}

// discardRemote closes a connection that must not serve anyone else: one
// with a request possibly still blocked on it, or one a stale job left.
func discardRemote(remote Remote) {
	if discardable, ok := remote.(discardableRemote); ok {
		_ = discardable.Discard()
		return
	}
	_ = remote.Close()
}

func (m *TransferManager) detachRemote(alias, id, target string) Remote {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	remote := m.remotes[key]
	delete(m.remotes, key)
	m.endIdlePeriodLocked(key)
	m.mutex.Unlock()
	return remote
}
