package remotesync_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

// contentReadCountingFileSystem は、ファイルの中身をメモリへ読んだ回数と、中身を
// 持たずに digest を求めた回数を数える。
type contentReadCountingFileSystem struct {
	storage.FileSystem
	mu           sync.Mutex
	contentReads map[string]int
	digests      int
}

func (f *contentReadCountingFileSystem) countRead(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.contentReads == nil {
		f.contentReads = map[string]int{}
	}
	f.contentReads[path]++
}

func (f *contentReadCountingFileSystem) ReadFile(path string) ([]byte, error) {
	f.countRead(path)
	return f.FileSystem.ReadFile(path)
}

func (f *contentReadCountingFileSystem) ReadFileLimited(path string, maximum int64) ([]byte, error) {
	f.countRead(path)
	return storage.ReadFileLimited(f.FileSystem, path, maximum)
}

func (f *contentReadCountingFileSystem) DigestFileLimited(path string, maximum int64) (string, error) {
	f.mu.Lock()
	f.digests++
	f.mu.Unlock()
	return storage.DigestFileLimited(f.FileSystem, path, maximum)
}

func (f *contentReadCountingFileSystem) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contentReads = nil
	f.digests = 0
}

func (f *contentReadCountingFileSystem) readsEndingWith(suffix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	reads := 0
	for path, count := range f.contentReads {
		if strings.HasSuffix(path, suffix) {
			reads += count
		}
	}
	return reads
}

// 変更があるかを見るだけの毎分の巡回と送信の下書きは、ファイルの中身をメモリへ読まず、
// digest だけを求める。背景画像が大きいと、読むたびにその大きさを割り当ててしまう。
func TestCheckingForLocalChangesDigestsFilesWithoutReadingThem(t *testing.T) {
	fileSystem := &contentReadCountingFileSystem{FileSystem: storage.OSFileSystem{}}
	machine := newInstallationOn(t, &fakeBucket{}, map[string]string{
		"config": "Host bastion\n",
		storage.BackgroundsDirectory + "/large.png": "a large background image",
	}, fileSystem)
	auto := autoFor(t, machine, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("first cycle = %+v", view)
	}

	fileSystem.reset()
	if view := auto.Poll(context.Background()); view.Phase != remotesync.AutoIdle {
		t.Fatalf("idle poll = %+v", view)
	}
	draft, err := machine.service.PushDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Added+draft.Modified+draft.Removed != 0 {
		t.Fatalf("draft = %+v, want nothing to send", draft)
	}
	if reads := fileSystem.readsEndingWith("large.png"); reads != 0 {
		t.Fatalf("the background was read into memory %d times", reads)
	}
	if fileSystem.digests == 0 {
		t.Fatal("no file was digested; the check did not look at the workspace")
	}
}
