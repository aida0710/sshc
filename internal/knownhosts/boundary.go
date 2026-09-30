package knownhosts

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

// ErrSymlinkPath は、シンボリックリンクを経由する known_hosts の読み書きを断る。
// 照合のための読み取り（ReadFile）はリンクをたどるが、鍵の保存と Known Hosts 画面は
// たどらない。dotfiles の管理で ~/.ssh/known_hosts をリンクにしていると当たる。
var ErrSymlinkPath = storage.ErrSymlinkPath

// ContentDigest は、確認から実行までの間に対象が変わっていないことを縛る印である。
func ContentDigest(contents []byte) string { return storage.Digest(contents) }

// IsExternalChange は、読んだときと書くときで known_hosts が変わっていたことを報告する。
func IsExternalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}
