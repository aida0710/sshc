package application

import (
	"errors"

	"sshc/internal/storage"
)

// このパッケージが外へ出しうるエラーの用語。
var (
	// ErrUnknownTransaction は、指定された取引が見つからないことを報告する。
	ErrUnknownTransaction = storage.ErrUnknownTransaction
	// ErrDirectoryNotEmpty は、中身のあるディレクトリの削除を断る。
	ErrDirectoryNotEmpty = storage.ErrDirectoryNotEmpty
	// ErrOutsideWorkspace は、~/.ssh の外を指す表記を断る。
	ErrOutsideWorkspace = storage.ErrOutsideWorkspace
	// ErrSymlinkPath は、シンボリックリンクを経由する表記を断る。
	ErrSymlinkPath = storage.ErrSymlinkPath
	// ErrNotRegularFile は、通常ファイルでないものへの書き込みを断る。
	ErrNotRegularFile = storage.ErrNotRegularFile
	// ErrMissingDirectory は、置き場が無いことを報告する。
	ErrMissingDirectory = storage.ErrMissingDirectory
	// ErrNotDirectory は、ディレクトリでないものをディレクトリとして扱う要求を断る。
	ErrNotDirectory = storage.ErrNotDirectory
)

// IsExternalChange は、読んだときと書くときで中身が変わっていたことを報告する。
func IsExternalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}
