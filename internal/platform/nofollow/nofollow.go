// Package nofollow は、symlink をたどらずにパスを開く。
//
// 最後の要素だけ O_NOFOLLOW にしても足りない。攻撃者は engine の起動後に親
// （ワークスペースの root を含む）を symlink に差し替えられる。開いた root の
// descriptor から openat で 1 要素ずつ降りることで、後続の lookup はすべて
// symlink をたどらずに開いた directory descriptor を基準にできる。
// storage のワークスペースと enginelock の state directory が同じ歩き方を使う。
package nofollow

import "errors"

// ErrSymlinkPath は、開く途中に symlink があった。
var ErrSymlinkPath = errors.New("path contains a symbolic link")
