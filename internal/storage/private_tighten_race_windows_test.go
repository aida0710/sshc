//go:build windows

package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"sshc/internal/platform/windowsacl"
)

// firstWriteRaceIterations は、まだ無いフォルダへ2つの書き手が同時に書く試行の回数である。
// 重なりは確率でしか起きないので、何度も繰り返す。
const firstWriteRaceIterations = 300

// まだ無いフォルダへ2つの書き手が同時に書く（MkdirAll のあと一時ファイルを作る）と、
// どちらも成功し、作った一時ファイルはどちらも非公開である。背景の画像を同時に2枚
// 受け取ったときの形である。
func TestTwoWritersCreatingTheSameDirectoryBothWritePrivateFiles(t *testing.T) {
	fileSystem := OSFileSystem{}
	base := filepath.Join(t.TempDir(), "state")
	if err := fileSystem.MkdirAll(base, DirectoryPermission); err != nil {
		t.Fatal(err)
	}
	var failures []error
	for iteration := range firstWriteRaceIterations {
		directory := filepath.Join(base, fmt.Sprint(iteration))
		var waiting sync.WaitGroup
		outcomes := make([]error, 2)
		for writer := range outcomes {
			waiting.Go(func() {
				outcomes[writer] = writeOnePrivateFile(fileSystem, directory)
			})
		}
		waiting.Wait()
		for writer, err := range outcomes {
			if err != nil {
				failures = append(failures, fmt.Errorf("iteration %d writer %d: %w", iteration, writer, err))
			}
		}
	}
	if len(failures) > 0 {
		t.Fatalf("%d of %d writes failed; first: %v", len(failures), 2*firstWriteRaceIterations, errors.Join(failures[:min(len(failures), 5)]...))
	}
}

func writeOnePrivateFile(fileSystem OSFileSystem, directory string) error {
	if err := fileSystem.MkdirAll(directory, DirectoryPermission); err != nil {
		return fmt.Errorf("MkdirAll: %w", err)
	}
	path, err := fileSystem.WriteTemp(directory, ".sshc-race-", FilePermission, []byte("race"))
	if err != nil {
		return fmt.Errorf("WriteTemp: %w", err)
	}
	restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
	if err != nil {
		return fmt.Errorf("inspect: %w", err)
	}
	if !restricted {
		return errors.New("inspect: the temporary file is not restricted to the current user")
	}
	return nil
}
