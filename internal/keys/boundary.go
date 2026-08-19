package keys

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
	// ErrMoveTargetExists は、行き先に既に何かあることを報告する。
	ErrMoveTargetExists = storage.ErrMoveTargetExists
	// ErrOutsideWorkspace は、~/.ssh の外を指す綴りを断る。
	ErrOutsideWorkspace = storage.ErrOutsideWorkspace
	// ErrSymlinkPath は、シンボリックリンクを経由する綴りを断る。
	ErrSymlinkPath = storage.ErrSymlinkPath
)

// IsExternalChange は、読んだときと書くときで中身が変わっていたことを報告する。
//
// **誰が変えたのかは分からない。** 別の端末かもしれず、手で開いたエディタかも
// しれない。分かるのは、いま書けば人の変更を踏むということだけである。
func IsExternalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}
