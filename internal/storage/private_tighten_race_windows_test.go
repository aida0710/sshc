//go:build windows

package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl"
)

// tightenRaceDuration は、締め直しと作成を重ねて走らせる時間である。重なりは確率でしか
// 起きないので、1回の試行ではなく時間で回す。
const tightenRaceDuration = 3 * time.Second

// tightenRaceSiblings は、先に置いておく配下の数である。SetSecurityInfo で DACL を
// 付け直すと、継承の伝搬が配下を1つずつ見て回る。配下を増やして伝搬の続く時間を延ばす。
const tightenRaceSiblings = 300

func prepareTightenRaceDirectory(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "state")
	fileSystem := OSFileSystem{}
	if err := fileSystem.MkdirAll(directory, DirectoryPermission); err != nil {
		t.Fatal(err)
	}
	for range tightenRaceSiblings {
		if _, err := fileSystem.WriteTemp(directory, ".sshc-sibling-", FilePermission, []byte("sibling")); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

// 同じフォルダを締め直す MkdirAll が重なっても、どちらも失敗しない。
func TestTighteningTheSameDirectoryConcurrentlySucceeds(t *testing.T) {
	directory := prepareTightenRaceDirectory(t)
	const tighteners = 4
	deadline := time.Now().Add(tightenRaceDuration)
	var waiting sync.WaitGroup
	var mutex sync.Mutex
	attempts, failures := 0, 0
	var firstFailure error
	for range tighteners {
		waiting.Go(func() {
			for time.Now().Before(deadline) {
				err := OSFileSystem{}.MkdirAll(directory, DirectoryPermission)
				mutex.Lock()
				attempts++
				if err != nil {
					failures++
					if firstFailure == nil {
						firstFailure = err
					}
				}
				mutex.Unlock()
			}
		})
	}
	waiting.Wait()
	if failures > 0 {
		t.Fatalf("%d of %d concurrent MkdirAll calls failed; first: %v", failures, attempts, firstFailure)
	}
	t.Logf("%d concurrent MkdirAll calls succeeded", attempts)
}

// フォルダを締め直す MkdirAll と、そのフォルダへの一時ファイルの作成が重なっても、作った
// 一時ファイルは作ったときも、締め直しが終わったあとも非公開である。
func TestATemporaryFileStaysPrivateWhileItsDirectoryIsTightened(t *testing.T) {
	directory := prepareTightenRaceDirectory(t)
	fileSystem := OSFileSystem{}
	stop := make(chan struct{})
	tightened := make(chan error, 1)
	go func() {
		count := 0
		for {
			select {
			case <-stop:
				tightened <- nil
				return
			default:
			}
			if err := fileSystem.MkdirAll(directory, DirectoryPermission); err != nil {
				tightened <- fmt.Errorf("MkdirAll after %d calls: %w", count, err)
				return
			}
			count++
		}
	}()

	var created []string
	attempts, createFailures := 0, 0
	var firstCreateFailure error
	for deadline := time.Now().Add(tightenRaceDuration); time.Now().Before(deadline); {
		attempts++
		path, err := fileSystem.WriteTemp(directory, ".sshc-race-", FilePermission, []byte("race"))
		if err != nil {
			createFailures++
			if firstCreateFailure == nil {
				firstCreateFailure = err
			}
			continue
		}
		created = append(created, path)
	}
	close(stop)
	if err := <-tightened; err != nil {
		t.Errorf("tightening the directory failed: %v", err)
	}

	// 伝搬が書き戻すのは、作成の確かめが済んだあとのこともある。締め直しを止めてから全部を見る。
	notPrivate := 0
	var firstNotPrivate error
	for _, path := range created {
		restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
		if err == nil && !restricted {
			err = errors.New("not restricted to the current user")
		}
		if err != nil {
			notPrivate++
			if firstNotPrivate == nil {
				firstNotPrivate = fmt.Errorf("%s: %w", filepath.Base(path), err)
			}
		}
	}
	if createFailures > 0 || notPrivate > 0 {
		t.Fatalf("of %d temporary files, %d failed to be created (first: %v) and %d of the created were not private afterwards (first: %v)",
			attempts, createFailures, firstCreateFailure, notPrivate, firstNotPrivate)
	}
	t.Logf("%d temporary files stayed private", len(created))
}
