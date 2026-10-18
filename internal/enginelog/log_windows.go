//go:build windows

package enginelog

import (
	"os"
	"path/filepath"

	"sshc/internal/platform/windowsacl"
)

// openPrivateAppend は、ログを本人だけが読める権限で作ってから、追記だけの権限で開き直す。
// 作るときのハンドルは権限を締めるために DELETE なども持つので、それを子へ渡すと、
// エンジンが動いているあいだほかのプログラムがログを読めなくなる。
func openPrivateAppend(path string) (*os.File, error) {
	directory, err := windowsacl.OpenPrivateDirectory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	created, err := windowsacl.OpenOrCreateFileAt(directory, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	if err := created.Close(); err != nil {
		return nil, err
	}
	return windowsacl.OpenAuthenticatedFileForAppend(path)
}

func openPrivateRead(path string) (*os.File, error) {
	return windowsacl.OpenAuthenticatedFileForRead(path)
}
