package knownhosts

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

// ContentDigest は、確認から実行までの間に対象が変わっていないことを縛る印である。
func ContentDigest(contents []byte) string { return storage.Digest(contents) }

// IsExternalChange は、読んだときと書くときで known_hosts が変わっていたことを報告する。
func IsExternalChange(err error) bool {
	var conflict *storage.ConflictError
	return errors.As(err, &conflict)
}
