package sftp

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	// thumbnailStallTimeout is how long a read may go without progress before its connection counts as dead. A
	// blocked SFTP read wakes only when its transport closes, and the host's other thumbnails wait behind it.
	thumbnailStallTimeout = 30 * time.Second
	// thumbnailRedialDelay spaces out the attempts at a host that could not be reached: each visible row would try
	// again otherwise, each one a dial that costs the phone's data and, on a slow route, tens of seconds.
	thumbnailRedialDelay = 30 * time.Second
)

// Thumbnailer makes the small pictures in a file list, from image files on hosts and on the engine's own disk.
//
// A host keeps the one SFTP connection the pane already uses. Thumbnails for a host are made one at a time, on a
// borrowed connection (RemotePool.Borrow), so that the list's other operations take the same connection at once rather
// than dial another, and a request the screen cancels stops reading between chunks without closing it. One image is
// decoded at a time, whichever pane it is for, which bounds the memory a folder of hostile files can take.
type Thumbnailer struct {
	service      Service
	cache        *thumbnailCache
	decoding     chan struct{}
	stallTimeout time.Duration
	now          func() time.Time

	mutex sync.Mutex
	hosts map[string]*thumbnailHost
}

// thumbnailHost is what the thumbnails of one host share.
type thumbnailHost struct {
	turn chan struct{}
	// redialAfter is when a dial may be tried again after one that failed. Only the holder of the turn touches it.
	redialAfter time.Time
}

func NewThumbnailer(service Service) *Thumbnailer {
	return &Thumbnailer{
		service:      service,
		cache:        newThumbnailCache(maxThumbnailCacheBytes),
		decoding:     make(chan struct{}, 1),
		stallTimeout: thumbnailStallTimeout,
		now:          time.Now,
		hosts:        map[string]*thumbnailHost{},
	}
}

// ForRemote makes the thumbnail of a regular file on a host, at the revision the listing showed. A symlink has none.
func (t *Thumbnailer) ForRemote(ctx context.Context, alias, remotePath, revision string) (Thumbnail, error) {
	if err := validateAlias(alias); err != nil {
		return Thumbnail{}, err
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Thumbnail{}, err
	}
	if revision == "" {
		return Thumbnail{}, ErrRevisionRequired
	}
	key := alias + "\x00" + cleaned + "\x00" + revision
	if outcome, found := t.cache.get(key); found {
		return outcome.thumbnail, outcome.refusal
	}
	host := t.host(alias)
	select {
	case host.turn <- struct{}{}:
	case <-ctx.Done():
		return Thumbnail{}, ctx.Err()
	}
	defer func() { <-host.turn }()
	// The request ahead in the queue may have been for the same file, from the other pane.
	if outcome, found := t.cache.get(key); found {
		return outcome.thumbnail, outcome.refusal
	}
	remote, err := t.connection(ctx, host, alias)
	if err != nil {
		return Thumbnail{}, err
	}
	defer func() { _ = remote.Close() }()
	source, err := t.readListedFile(ctx, remote, listedFile{path: cleaned, revision: revision})
	if err != nil {
		return t.finished(key, Thumbnail{}, err)
	}
	return t.render(ctx, key, source)
}

// ForLocal makes the thumbnail of a regular file on the engine's disk. Links and files under the operating system's
// privacy protection are refused, as they are when read for a preview.
func (t *Thumbnailer) ForLocal(ctx context.Context, localPath, revision string) (Thumbnail, error) {
	if revision == "" {
		return Thumbnail{}, ErrRevisionRequired
	}
	key := LocalAlias + "\x00" + localPath + "\x00" + revision
	if outcome, found := t.cache.get(key); found {
		return outcome.thumbnail, outcome.refusal
	}
	source, err := readLocalThumbnailSource(ctx, listedFile{path: localPath, revision: revision})
	if err != nil {
		return t.finished(key, Thumbnail{}, err)
	}
	return t.render(ctx, key, source)
}

// listedFile is a file as the listing on screen showed it.
type listedFile struct {
	path     string
	revision string
}

func readLocalThumbnailSource(ctx context.Context, listed listedFile) (thumbnailSource, error) {
	download, err := OpenLocalDownload(listed.path)
	if err != nil {
		return thumbnailSource{}, err
	}
	defer func() { _ = download.File.Close() }()
	if download.Revision != listed.revision {
		return thumbnailSource{}, ErrConflict
	}
	info, err := download.File.Stat()
	if err != nil {
		return thumbnailSource{}, err
	}
	return readThumbnailSource(ctx, download.File, info.Size())
}

func (t *Thumbnailer) render(ctx context.Context, key string, source thumbnailSource) (Thumbnail, error) {
	select {
	case t.decoding <- struct{}{}:
	case <-ctx.Done():
		return Thumbnail{}, ctx.Err()
	}
	defer func() { <-t.decoding }()
	thumbnail, err := renderThumbnail(source)
	return t.finished(key, thumbnail, err)
}

// finished remembers the picture and the refusals that hold for this revision of the file. A failed connection, a
// change under the read or a cancelled request says nothing about the file and is tried again next time.
func (t *Thumbnailer) finished(key string, thumbnail Thumbnail, err error) (Thumbnail, error) {
	if err == nil || errors.Is(err, ErrPreviewType) || errors.Is(err, ErrPreviewTooLarge) {
		t.cache.put(key, thumbnailOutcome{thumbnail: thumbnail, refusal: err})
	}
	return thumbnail, err
}

func (t *Thumbnailer) host(alias string) *thumbnailHost {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	host, found := t.hosts[alias]
	if !found {
		host = &thumbnailHost{turn: make(chan struct{}, 1)}
		t.hosts[alias] = host
	}
	return host
}

// connection lends the connection the host already has. With none open, the pane's connection has gone idle and closed
// (RemoteIdleTimeout), so one is opened in its place, into the pool. A host answered with a one-time code is never
// dialled from here: the code may already have been used in this window, and the pane's next operation would be
// refused for it.
func (t *Thumbnailer) connection(ctx context.Context, host *thumbnailHost, alias string) (Remote, error) {
	if t.service.Borrow == nil {
		return nil, ErrUnavailable
	}
	remote, err := t.service.Borrow(ctx, alias)
	if !errors.Is(err, ErrNoOpenConnection) {
		return remote, err
	}
	if t.now().Before(host.redialAfter) || (t.service.ConnectionLimit != nil && t.service.ConnectionLimit(alias) > 0) {
		return nil, ErrNoOpenConnection
	}
	opened, err := t.service.open(ctx, alias)
	if err != nil {
		if ctx.Err() == nil {
			host.redialAfter = t.now().Add(thumbnailRedialDelay)
		}
		return nil, err
	}
	// Closing returns it to the pool, where the pane's next operation finds it too.
	if err := opened.Close(); err != nil {
		return nil, err
	}
	return t.service.Borrow(ctx, alias)
}

// readListedFile reads a file on a connection and makes sure that what was read is what the listing showed.
func (t *Thumbnailer) readListedFile(ctx context.Context, remote Remote, listed listedFile) (thumbnailSource, error) {
	watchdog := newThumbnailWatchdog(t.stallTimeout, remote)
	defer watchdog.timer.Stop()
	before, err := remote.Lstat(listed.path)
	if err != nil {
		return thumbnailSource{}, err
	}
	if !before.Mode().IsRegular() {
		return thumbnailSource{}, ErrNotRegularFile
	}
	if metadataRevision(before) != listed.revision {
		return thumbnailSource{}, ErrConflict
	}
	file, err := remote.Open(listed.path)
	if err != nil {
		return thumbnailSource{}, err
	}
	defer func() { _ = file.Close() }()
	source, err := readThumbnailSource(ctx, watchedReader{file: file, watchdog: watchdog}, before.Size())
	if err != nil {
		return thumbnailSource{}, err
	}
	after, err := remote.Lstat(listed.path)
	if err != nil {
		return thumbnailSource{}, err
	}
	if metadataRevision(after) != listed.revision {
		return thumbnailSource{}, ErrConflict
	}
	return source, nil
}

// thumbnailWatchdog closes the connection of a read that has stopped making progress.
type thumbnailWatchdog struct {
	timeout time.Duration
	timer   *time.Timer
}

func newThumbnailWatchdog(timeout time.Duration, remote Remote) *thumbnailWatchdog {
	return &thumbnailWatchdog{timeout: timeout, timer: time.AfterFunc(timeout, func() {
		if discardable, ok := remote.(discardableRemote); ok {
			_ = discardable.Discard()
		}
	})}
}

// watchedReader gives the watchdog more time with every read that returns.
type watchedReader struct {
	file     io.Reader
	watchdog *thumbnailWatchdog
}

func (r watchedReader) Read(buffer []byte) (int, error) {
	read, err := r.file.Read(buffer)
	r.watchdog.timer.Reset(r.watchdog.timeout)
	return read, err
}
