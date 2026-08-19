package application

import (
	"errors"

	"sshc/internal/storage"
)

// このパッケージが外へ出しうるエラーの語彙。
//
// **HTTP 層が internal/storage を名指ししないためである。** 永続化層のエラー名が
// そのまま外向きの応答に対応していると、保存の都合で付けた名前を変えるだけで
// HTTP の契約が動く。ここに別名を置くことで、語彙の持ち主はこのサービスになる。
//
// 別名であって包み直しではない。errors.Is はどちらの綴りでも通る。
var (
	// ErrUnknownTransaction は、名指しされた取引が見つからないことを報告する。
	ErrUnknownTransaction = storage.ErrUnknownTransaction
	// ErrDirectoryNotEmpty は、中身のあるディレクトリの削除を断る。
	ErrDirectoryNotEmpty = storage.ErrDirectoryNotEmpty
	// ErrOutsideWorkspace は、~/.ssh の外を指す綴りを断る。
	ErrOutsideWorkspace = storage.ErrOutsideWorkspace
	// ErrSymlinkPath は、シンボリックリンクを経由する綴りを断る。
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
