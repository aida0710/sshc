// Package nativepath は、このアプリケーションが動いているファイルシステムの
// 文法でパスを判断する。
//
// 設定のパスは、スラッシュ区切りの識別子ではない。OpenSSH の Include が
// スラッシュで書かれていても、それが指すのは OS 上のファイルであり、`C:\Users`
// も `\\server\share` も絶対パスである。`path` パッケージでそれを見ると、
// Windows のホームはどれも「絶対パスではない」ことになり、設定はひとつも
// 読めない。
//
// ここにあるのは、標準ライブラリの `path/filepath` が応答しない三つだけである。
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

// Contains は、candidate が root そのものか、その下にあるかを言う。
//
// 素の文字列前置比較ではなく filepath.Rel を通すのは、そこにボリュームの一致と
// Windows の大小文字同一視が既に入っているからである。前置比較だけでは
// `~/.ssh-other` が `~/.ssh` の中になり、`C:\x` と `D:\x` が区別されない。
func Contains(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Identity は、同じファイルを指す二つの表記が等しくなる鍵を返す。
//
// Include の重複と循環を数えるために要る。Windows では `C:\Users\A\.ssh\config`
// と `c:\users\a\.ssh\CONFIG` は同じファイルなので、別々に読み込めば同じ内容が
// 二重に現れ、循環はいつまでも見つからない。
func Identity(path string) string {
	return foldIdentity(filepath.Clean(path))
}
