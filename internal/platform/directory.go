package platform

import (
	"errors"
	"path/filepath"
	"strings"
)

// ディレクトリの指定を断る理由。
var (
	// ErrDirectoryRelative は、home からも / からも辿れない指定を断る。
	//
	// 相対パスの意味は、それを解釈するプロセスの居場所で変わる。ここが
	// 受け取るのは保存される設定であり、保存されたものが読むたびに別の場所を
	// 指すのは、設定として成立していない。
	//
	// Windows の `~\work` もここで断る。保存した綴りは同期先の macOS や Linux
	// でも読まれ、そこでは `\` がファイル名の一部になる。どの OS でも home の
	// 下を指す `~/` だけを受け付ける。
	ErrDirectoryRelative = errors.New("the directory must be absolute or start with ~/")
	// ErrDirectoryUser は、`~someone` の形を断る。別のユーザーの home の表記を
	// 知る手段をこのアプリケーションは持たない。
	ErrDirectoryUser = errors.New("another user's home directory cannot be resolved")
)

// ResolveUnderHome は、設定に書かれたディレクトリを絶対パスにする。
//
// 空文字は空文字のまま返す。「書かれていない」は「/」ではない。呼び出し側が
// 自分の既定へ倒せるように、区別を残したまま返す。
func ResolveUnderHome(path, home string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", nil
	}
	switch {
	case trimmed == "~":
		return filepath.Clean(home), nil
	case strings.HasPrefix(trimmed, "~/"):
		return filepath.Join(home, trimmed[2:]), nil
	case strings.HasPrefix(trimmed, `~\`):
		return "", ErrDirectoryRelative
	case strings.HasPrefix(trimmed, "~"):
		return "", ErrDirectoryUser
	case !filepath.IsAbs(trimmed):
		return "", ErrDirectoryRelative
	}
	return filepath.Clean(trimmed), nil
}
