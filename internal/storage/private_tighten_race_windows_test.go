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

// まだ無いフォルダへ2つの書き手が同時に書いても、どちらも成功し、作ったフォルダと一時
// ファイルは非公開のままである。背景の画像を同時に2枚受け取ったときの形で、各書き手は
// Workspace.EnsureDirectory と同じく既にある親を締め直してから、フォルダを作って書く。
// 親を締め直す SetSecurityInfo は、継承を配下へ伝搬する。
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
				outcomes[writer] = writeOnePrivateFile(fileSystem, base, directory)
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

func writeOnePrivateFile(fileSystem OSFileSystem, parent, directory string) error {
	if err := fileSystem.MkdirAll(parent, DirectoryPermission); err != nil {
		return fmt.Errorf("MkdirAll of the parent: %w", err)
	}
	if err := fileSystem.MkdirAll(directory, DirectoryPermission); err != nil {
		return fmt.Errorf("MkdirAll: %w", err)
	}
	path, err := fileSystem.WriteTemp(directory, ".sshc-race-", FilePermission, []byte("race"))
	if err != nil {
		return fmt.Errorf("WriteTemp: %w", err)
	}
	for _, created := range []string{directory, path} {
		restricted, err := windowsacl.IsRestrictedToCurrentUser(created)
		if err != nil {
			return fmt.Errorf("inspect %s: %w", filepath.Base(created), err)
		}
		if !restricted {
			return fmt.Errorf("inspect %s: not restricted to the current user", filepath.Base(created))
		}
	}
	return nil
}
