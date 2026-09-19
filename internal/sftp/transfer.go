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

// TransferManager serializes requests that operate on the same resumable part file.
// Different files remain independent so a large queue can make bounded parallel progress.
// TransferManager は、engine が抱える転送の全体である。3 つの関心ごとに 1 つずつ
// lock を持ち、field はその lock の下に並べる。
//
//   - データプレーン（mutex）: 対象ごとの operation lock、開いた remote、
//     prepared download の spool。upload_plane.go／download_plane.go／spool.go。
//   - job 台帳（jobsMutex）: queue の記録・順序・設定・永続化。jobs.go／
//     jobs_settings.go／queue_store.go。
//   - remote worker（remoteJobsMutex）: engine 内で走る copy／move／delete／
//     put／get。remote_jobs.go。
//
// `*Owned` の API は job に属する転送で、データプレーンを進めながら台帳の
// record を更新する。両方の lock を取る唯一の層である。
type TransferManager struct {
	Service *Service

	// データプレーン。mutex が守る。
	mutex     sync.Mutex
	locks     map[string]*transferLock
	remotes   map[string]Remote
	downloads map[string]preparedDownloadCache
	spoolDir  string
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

	// remote worker。remoteJobsMutex が守る。
	remoteJobsMutex sync.Mutex
	remoteCancels   map[string]context.CancelFunc
	remoteWorkers   sync.WaitGroup
}

type transferLock struct {
	mutex sync.RWMutex
	refs  int
}

func NewTransferManager(service *Service) *TransferManager {
	manager := &TransferManager{Service: service, locks: make(map[string]*transferLock), remotes: make(map[string]Remote), downloads: make(map[string]preparedDownloadCache), spoolDir: downloadSpoolDirectory(), remoteCancels: make(map[string]context.CancelFunc)}
	manager.ConfigureJobs(DefaultTransferConcurrency, time.Now)
	return manager
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
		downloads := make([]*PreparedDownload, 0, len(m.downloads))
		for id, cached := range m.downloads {
			delete(m.downloads, id)
			downloads = append(downloads, cached.download)
		}
		m.mutex.Unlock()
		m.remoteJobsMutex.Lock()
		for _, cancel := range m.remoteCancels {
			cancel()
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

func (m *TransferManager) transferRemote(ctx context.Context, alias, id, target string) (Remote, error) {
	key := transferRemoteKey(alias, id, target)
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return nil, ErrUnavailable
	}
	remote := m.remotes[key]
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
	m.mutex.Unlock()
	return remote
}
