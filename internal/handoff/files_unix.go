//go:build unix

package handoff

import (
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"sshc/internal/platform/nofollow"
	"sshc/internal/storage"
)

func defaultWriteOperations() writeOperations {
	return writeOperations{ensureDirectory: ensureHandoffDirectory}
}

func defaultHandoffFileOperations() handoffFileOperations {
	return handoffFileOperations{open: openHandoffFile, remove: removeHandoffFile}
}

// openHandoffFile は、ワークスペースのほかの非公開ファイルと同じく、途中も最後も
// symlink をたどらずに開く。エラーにはパスを付け、CLI が何を開けなかったかを示す。
func openHandoffFile(path string) (*os.File, error) {
	file, err := nofollow.OpenRegular(path)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return file, nil
}

// removeHandoffFile は、読み終えた handle を閉じてから、symlink をたどらずに開いた
// 親ディレクトリの descriptor から名前を消す。
func removeHandoffFile(file *os.File, path string) error {
	if err := file.Close(); err != nil {
		return err
	}
	directory, err := nofollow.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return unix.Unlinkat(int(directory.Fd()), filepath.Base(path), 0)
}

// ensureHandoffDirectory は、storage の非公開ディレクトリと同じ歩き方で作る。
// symlink の先で mkdir や chmod をしてから失敗することがない。
func ensureHandoffDirectory(path string) error {
	directory, err := nofollow.OpenOrCreateDirectory(path, storage.DirectoryPermission)
	if err != nil {
		return err
	}
	return directory.Close()
}
