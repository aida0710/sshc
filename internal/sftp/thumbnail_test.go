package sftp

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// thumbnailHostFixture is a host with a fake SFTP connection behind a real pool. It counts what the thumbnails cost.
type thumbnailHostFixture struct {
	pool    *RemotePool
	service Service
	dials   atomic.Int32
	remote  *thumbnailRemote
	refuse  atomic.Bool
}

func newThumbnailHostFixture(files map[string]*thumbnailFile) *thumbnailHostFixture {
	fixture := &thumbnailHostFixture{remote: &thumbnailRemote{files: files}}
	fixture.pool = NewRemotePool(func(context.Context, string) (RemoteTarget, error) {
		return RemoteTarget{Identity: "host", Open: func(context.Context) (Remote, error) {
			if fixture.refuse.Load() {
				return nil, errors.New("connection refused")
			}
			fixture.dials.Add(1)
			return fixture.remote, nil
		}}, nil
	})
	fixture.service = Service{Open: fixture.pool.Open, Borrow: fixture.pool.Borrow}
	return fixture
}

// connectPane opens the connection a listing would have left in the pool.
func (f *thumbnailHostFixture) connectPane(t *testing.T) {
	t.Helper()
	remote, err := f.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Close(); err != nil {
		t.Fatal(err)
	}
}

type thumbnailRemote struct {
	Remote
	mutex                     sync.Mutex
	files                     map[string]*thumbnailFile
	opens                     []string
	closed                    bool
	onRead                    func(path string) // Runs before each read of a file.
	concurrent, maxConcurrent atomic.Int32
}

func (r *thumbnailRemote) Alive() bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return !r.closed
}

func (r *thumbnailRemote) Close() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.closed = true
	return nil
}

func (r *thumbnailRemote) Lstat(path string) (fs.FileInfo, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	file, found := r.files[path]
	if !found {
		return nil, fs.ErrNotExist
	}
	return file.info(), nil
}

func (r *thumbnailRemote) Open(path string) (io.ReadCloser, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	file, found := r.files[path]
	if !found {
		return nil, fs.ErrNotExist
	}
	r.opens = append(r.opens, path)
	return &thumbnailOpenFile{remote: r, path: path, reader: bytes.NewReader(file.contents)}, nil
}

func (r *thumbnailRemote) openCount() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return len(r.opens)
}

type thumbnailOpenFile struct {
	remote *thumbnailRemote
	path   string
	reader *bytes.Reader
	once   sync.Once
}

func (f *thumbnailOpenFile) Read(buffer []byte) (int, error) {
	f.once.Do(func() {
		if concurrent := f.remote.concurrent.Add(1); concurrent > f.remote.maxConcurrent.Load() {
			f.remote.maxConcurrent.Store(concurrent)
		}
	})
	if f.remote.onRead != nil {
		f.remote.onRead(f.path)
	}
	return f.reader.Read(buffer)
}

func (f *thumbnailOpenFile) Close() error {
	f.remote.concurrent.Add(-1)
	return nil
}

type thumbnailFile struct {
	contents []byte
	modified time.Time
	mode     fs.FileMode
}

func (f *thumbnailFile) info() fs.FileInfo {
	return thumbnailFileInfo{size: int64(len(f.contents)), modified: f.modified, mode: f.mode}
}

func (f *thumbnailFile) revision() string { return metadataRevision(f.info()) }

type thumbnailFileInfo struct {
	fs.FileInfo
	size     int64
	modified time.Time
	mode     fs.FileMode
}

func (i thumbnailFileInfo) Size() int64        { return i.size }
func (i thumbnailFileInfo) ModTime() time.Time { return i.modified }
func (i thumbnailFileInfo) Mode() fs.FileMode  { return i.mode }
func (i thumbnailFileInfo) Sys() any           { return nil }

func pictureFile(t *testing.T) *thumbnailFile {
	t.Helper()
	return &thumbnailFile{contents: encodedPNG(t, filledPicture(32, 32, color.RGBA{R: 200, A: 255})), modified: time.Unix(1_800_000_000, 0), mode: 0o644}
}

func TestAThumbnailIsMadeOnceForARevisionAndAgainWhenTheFileChanges(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	thumbnails := NewThumbnailer(fixture.service)

	first, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision()); err != nil {
		t.Fatal(err)
	}
	if opens := fixture.remote.openCount(); opens != 1 {
		t.Fatalf("read the file %d times for one revision", opens)
	}
	if first.ContentType != "image/jpeg" || len(first.Contents) == 0 {
		t.Fatalf("thumbnail = %q, %d bytes", first.ContentType, len(first.Contents))
	}

	fixture.remote.mutex.Lock()
	file.modified = file.modified.Add(time.Hour)
	fixture.remote.mutex.Unlock()
	if _, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision()); err != nil {
		t.Fatal(err)
	}
	if opens := fixture.remote.openCount(); opens != 2 {
		t.Fatalf("a changed file was read %d times in all", opens)
	}
}

func TestAFileThatHasNoPictureIsNotReadAgain(t *testing.T) {
	file := &thumbnailFile{contents: []byte("not a picture"), modified: time.Unix(1_800_000_000, 0), mode: 0o644}
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	thumbnails := NewThumbnailer(fixture.service)
	for range 2 {
		if _, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision()); !errors.Is(err, ErrPreviewType) {
			t.Fatalf("err = %v", err)
		}
	}
	if opens := fixture.remote.openCount(); opens != 1 {
		t.Fatalf("opened %d times", opens)
	}
}

func TestAFileChangedSinceTheListingIsNotReadAndNotRemembered(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	thumbnails := NewThumbnailer(fixture.service)
	stale := "stale-revision"
	if _, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if opens := fixture.remote.openCount(); opens != 0 {
		t.Fatalf("opened %d times", opens)
	}
	if _, found := thumbnails.cache.get("edge\x00/a.png\x00" + stale); found {
		t.Fatal("remembered a conflict")
	}
}

func TestASymlinkHasNoThumbnail(t *testing.T) {
	link := &thumbnailFile{contents: []byte("/secret.png"), modified: time.Unix(1_800_000_000, 0), mode: fs.ModeSymlink | 0o777}
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": link})
	fixture.connectPane(t)
	if _, err := NewThumbnailer(fixture.service).ForRemote(context.Background(), "edge", "/a.png", link.revision()); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("err = %v", err)
	}
}

func TestThumbnailsOfAHostAreMadeOneAtATimeOnTheOneConnection(t *testing.T) {
	files := map[string]*thumbnailFile{}
	paths := []string{"/a.png", "/b.png", "/c.png", "/d.png", "/e.png", "/f.png"}
	for _, path := range paths {
		files[path] = pictureFile(t)
	}
	fixture := newThumbnailHostFixture(files)
	fixture.connectPane(t)
	thumbnails := NewThumbnailer(fixture.service)
	var waiting sync.WaitGroup
	for _, path := range paths {
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			if _, err := thumbnails.ForRemote(context.Background(), "edge", path, files[path].revision()); err != nil {
				t.Error(err)
			}
		}()
	}
	waiting.Wait()
	if dials := fixture.dials.Load(); dials != 1 {
		t.Fatalf("dialled %d times", dials)
	}
	if most := fixture.remote.maxConcurrent.Load(); most != 1 {
		t.Fatalf("%d files were read at once", most)
	}
}

func TestAnOperationDuringAThumbnailTakesTheSameConnection(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	reading, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	fixture.remote.onRead = func(string) {
		once.Do(func() { close(reading); <-release })
	}
	finished := make(chan error, 1)
	go func() {
		_, err := NewThumbnailer(fixture.service).ForRemote(context.Background(), "edge", "/a.png", file.revision())
		finished <- err
	}()
	<-reading
	listing, err := fixture.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	if dials := fixture.dials.Load(); dials != 1 {
		t.Fatalf("the listing dialled a connection of its own: %d", dials)
	}
	_ = listing.Close()
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestAThumbnailBorrowsTheConnectionAnOperationIsUsing(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	transfer, err := fixture.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	defer transfer.Close()
	if _, err := NewThumbnailer(fixture.service).ForRemote(context.Background(), "edge", "/a.png", file.revision()); err != nil {
		t.Fatal(err)
	}
	if dials := fixture.dials.Load(); dials != 1 {
		t.Fatalf("dialled %d times", dials)
	}
}

func TestACancelledThumbnailLeavesTheConnectionOpen(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	ctx, cancel := context.WithCancel(context.Background())
	fixture.remote.onRead = func(string) { cancel() }
	if _, err := NewThumbnailer(fixture.service).ForRemote(ctx, "edge", "/a.png", file.revision()); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if !fixture.remote.Alive() {
		t.Fatal("cancelling a thumbnail closed the pane's connection")
	}
	next, err := fixture.pool.Open(context.Background(), "edge")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if dials := fixture.dials.Load(); dials != 1 {
		t.Fatalf("the next operation dialled again: %d", dials)
	}
}

func TestAPaneWhoseConnectionWentIdleGetsOneConnectionBack(t *testing.T) {
	file := pictureFile(t)
	other := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file, "/b.png": other})
	thumbnails := NewThumbnailer(fixture.service)
	for path, listed := range map[string]*thumbnailFile{"/a.png": file, "/b.png": other} {
		if _, err := thumbnails.ForRemote(context.Background(), "edge", path, listed.revision()); err != nil {
			t.Fatal(err)
		}
	}
	if dials := fixture.dials.Load(); dials != 1 {
		t.Fatalf("dialled %d times", dials)
	}
}

func TestAHostAnsweredWithAOneTimeCodeIsNotDialled(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.service.ConnectionLimit = func(string) int { return 1 }
	if _, err := NewThumbnailer(fixture.service).ForRemote(context.Background(), "edge", "/a.png", file.revision()); !errors.Is(err, ErrNoOpenConnection) {
		t.Fatalf("err = %v", err)
	}
	if dials := fixture.dials.Load(); dials != 0 {
		t.Fatalf("dialled %d times", dials)
	}
}

func TestAHostThatCannotBeReachedIsNotDialledForEveryRow(t *testing.T) {
	file := pictureFile(t)
	attempts := 0
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.service.Open = func(ctx context.Context, alias string) (Remote, error) {
		attempts++
		return nil, errors.New("connection refused")
	}
	thumbnails := NewThumbnailer(fixture.service)
	clock := time.Unix(1_800_000_000, 0)
	thumbnails.now = func() time.Time { return clock }
	for range 3 {
		_, _ = thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision())
	}
	if attempts != 1 {
		t.Fatalf("dialled %d times", attempts)
	}
	clock = clock.Add(thumbnailRedialDelay)
	_, _ = thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision())
	if attempts != 2 {
		t.Fatalf("did not try again after the delay: %d", attempts)
	}
}

func TestAReadThatStopsRespondingClosesTheConnection(t *testing.T) {
	file := pictureFile(t)
	fixture := newThumbnailHostFixture(map[string]*thumbnailFile{"/a.png": file})
	fixture.connectPane(t)
	unblocked := make(chan struct{})
	fixture.remote.onRead = func(string) { <-unblocked }
	thumbnails := NewThumbnailer(fixture.service)
	thumbnails.stallTimeout = time.Millisecond
	finished := make(chan error, 1)
	go func() {
		_, err := thumbnails.ForRemote(context.Background(), "edge", "/a.png", file.revision())
		finished <- err
	}()
	for fixture.remote.Alive() {
		time.Sleep(time.Millisecond)
	}
	close(unblocked)
	<-finished
}

func TestALocalFileMakesAThumbnailAndALinkDoesNot(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "a.png")
	if err := os.WriteFile(path, encodedPNG(t, filledPicture(16, 16, color.Black)), 0o644); err != nil {
		t.Fatal(err)
	}
	download, err := OpenLocalDownload(filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	revision := download.Revision
	_ = download.File.Close()
	thumbnails := NewThumbnailer(Service{})
	thumbnail, err := thumbnails.ForLocal(context.Background(), filepath.ToSlash(path), revision)
	if err != nil {
		t.Fatal(err)
	}
	if thumbnail.ContentType != "image/jpeg" {
		t.Fatalf("type = %q", thumbnail.ContentType)
	}
	if _, err := thumbnails.ForLocal(context.Background(), filepath.ToSlash(path), "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	link := filepath.Join(directory, "link.png")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("symlinks are not available here")
	}
	if _, err := thumbnails.ForLocal(context.Background(), filepath.ToSlash(link), revision); err == nil {
		t.Fatal("made a thumbnail through a link")
	}
}

func TestTheCacheDropsTheLeastRecentlyUsedOverItsBudget(t *testing.T) {
	cache := newThumbnailCache(100)
	entry := func(size int) thumbnailOutcome {
		return thumbnailOutcome{thumbnail: Thumbnail{Contents: make([]byte, size)}}
	}
	cache.put("a", entry(40))
	cache.put("b", entry(40))
	if _, found := cache.get("a"); !found {
		t.Fatal("a is gone too early")
	}
	cache.put("c", entry(40))
	if _, found := cache.get("b"); found {
		t.Fatal("b should be the first out")
	}
	if _, found := cache.get("a"); !found {
		t.Fatal("a was used more recently than b")
	}
	if cache.used > cache.capacity {
		t.Fatalf("used %d of %d", cache.used, cache.capacity)
	}
}
