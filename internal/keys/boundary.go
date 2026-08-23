package keys

import (
	"errors"

	"sshc/internal/storage"
)

// このパッケージが外へ出しうるエラーの用語。
//
// HTTP 層が internal/storage を指定しないためである。永続化層のエラー名が
// そのまま外向きの応答に対応していると、保存の都合で付けた名前を変えるだけで
// HTTP の契約が動く。ここに別名を置くことで、用語の持ち主はこのサービスになる。
//
// 別名であって包み直しではない。errors.Is はどちらの表記でも通る。
var (
	// ErrMoveTargetExists は、行き先に既に何かあることを報告する。
	ErrMoveTargetExists = storage.ErrMoveTargetExists
	// ErrOutsideWorkspace は、~/.ssh の外を指す表記を断る。
	ErrOutsideWorkspace = storage.ErrOutsideWorkspace
	// ErrSymlinkPath は、シンボリックリンクを経由する表記を断る。
	ErrSymlinkPath = storage.ErrSymlinkPath
)

// IsExternalChange は、読んだときと書くときで中身が変わっていたことを報告する。
//
// 誰が変えたのかは分からない。別の端末かもしれず、手で開いたエディタかも
// しれない。分かるのは、いま書けばユーザーの変更を踏むということだけである。
func IsExternalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}
