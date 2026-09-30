// Package nativepath は、このアプリケーションが動いているファイルシステムの
// 文法でパスを判断する。
//
// 設定のパスは、スラッシュ区切りの識別子ではない。OpenSSH の Include が
// スラッシュで書かれていても、それが指すのは OS 上のファイルであり、`C:\Users`
// も `\\server\share` も絶対パスである。`path` パッケージでそれを見ると、
// Windows のホームはどれも「絶対パスではない」ことになり、設定はひとつも
// 読めない。
//
// ここにあるのは、標準ライブラリの `path/filepath` が応答しない判断だけである。
// 残りはすべて `filepath` に任せる。ボリュームの扱いも、Windows の大小文字
// 同一視も、そちらが持っているからだ。
package nativepath

import (
	"path/filepath"
	"strings"
)

// Supported は、このアプリケーションが実際に触れる絶対パスかどうかを言う。
//
// filepath.IsAbs より狭い。Win32 の device 名前空間と拡張名前空間
// (`\\?\`、`\\.\`、`\??\`) を拒む。これらは、他のすべての層が適用している
// 正規化・大小文字・包含の規則をそのまま適用できない表記であり、受け入れれば
// 「ワークスペースの中か」を決める判断だけが別の文法で行われることになる。
//
// 名前空間の判定はどの OS でも同じように行う。Windows で書かれた設定を Unix で
// 読んだときにも、同じ理由で同じように拒むためである。
func Supported(path string) bool {
	if path == "" || strings.IndexByte(path, 0) >= 0 || deviceNamespace(path) {
		return false
	}
	return filepath.IsAbs(path)
}

func deviceNamespace(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	for _, prefix := range []string{`\\?\`, `\\.\`, `\??\`} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// Relative は、candidate が root そのものかその下にあるとき、root からの
// ネイティブ区切りの相対パスを返す。root そのものは "." になる。外なら偽を返す。
//
// 素の文字列前置比較ではなく filepath.Rel を通すのは、そこにボリュームの一致と
// Windows の大小文字同一視が既に入っているからである。前置比較だけでは
// `~/.ssh-other` が `~/.ssh` の中になり、`C:\x` と `D:\x` が区別されない。
// Rel が絶対パスを返すこと（Windows で要素に `C:` を含むパス）も外として扱う。
// `..foo` のような名前は親への参照と取り違えない。
func Relative(root, candidate string) (string, bool) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}

// RelativeBelow は、root 自身を除いて、candidate が root の下にあるときだけ
// Relative と同じ相対パスを返す。
func RelativeBelow(root, candidate string) (string, bool) {
	relative, ok := Relative(root, candidate)
	if !ok || relative == "." {
		return "", false
	}
	return relative, true
}

// Contains は、candidate が root そのものか、その下にあるかを言う。
func Contains(root, candidate string) bool {
	_, ok := Relative(root, candidate)
	return ok
}

// RelativeSlash は、root の下にある absolute を root からの slash 区切りの相対パスに
// する。root の外（root 自身を含む）なら偽を返す。鍵の絶対パスを vault の保存値へ
// 対応づける経路のように、ワークスペース相対の識別子が要る場所が使う。
func RelativeSlash(root, absolute string) (string, bool) {
	relative, ok := RelativeBelow(root, absolute)
	if !ok {
		return "", false
	}
	return filepath.ToSlash(relative), true
}

// Identity は、同じファイルを指す二つの表記が等しくなる鍵を返す。
//
// Include の重複と循環を数えるために要る。Windows では `C:\Users\A\.ssh\config`
// と `c:\users\a\.ssh\CONFIG` は同じファイルなので、別々に読み込めば同じ内容が
// 二重に現れ、循環はいつまでも見つからない。
func Identity(path string) string {
	return foldIdentity(filepath.Clean(path))
}

// MatchPrefix は、text が path と同じパスの表記で始まるかを言い、一致した部分の
// text でのバイト数を返す。
//
// 表記の比べ方は Identity と同じである。Windows では大小文字と区切り文字
// （/ と \）の違いを同一視する。Unix では表記がそのまま一致するときだけである。
// 文中のパスを探す側（ホームを伏せる処理）も、Include の重複の判定と同じ規則で
// 同じパスを見分けられるよう、ここに置く。
func MatchPrefix(text, path string) (int, bool) {
	return matchPrefix(text, path)
}
