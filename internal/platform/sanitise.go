package platform

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"sshc/internal/platform/nativepath"
)

// SanitiseHomePaths は、テキスト中のユーザーのホームディレクトリを "~" に書き換える。
//
// 認証テストの結果は、鍵を読めなかった理由として IdentityFile の絶対パスを含む。
// 鍵の登録でリモートのシェルから受け取った stderr にも、同じ処理をかける。
// そのまま返すと、このアプリケーションを動かしているユーザーのアカウント名を
// レスポンス本文へ運んでしまう。テキスト自体は引き続き表示する。失敗を理解する
// ためにユーザーが必要とするからだ。取り除くのは、アカウントを特定する部分だけである。
//
// 書き換えるのは、ホームがひとつのパスの先頭として現れたところだけである。直前は
// テキストの先頭かパスの外側の文字、直後は区切り文字かパスの外側の文字かテキストの
// 終わりでなければならない。ホームが /home/al のとき、/home/alice を ~ice に
// 書き換えると、シェルの表記では「ice というユーザーのホーム」という別のパスに
// 読める。/mnt/snap/home/al を /mnt/snap~ にするのも同じく読み違えを生む。
//
// 表記の比べ方は nativepath に合わせる。Windows では大小文字の違う綴りも同じ
// ホームなので、それも伏せる。
//
// ホームが空またはルート（/ や C:\）の場合は無視する。ルートを書き換えれば、
// 何も隠さないまま出力中のあらゆる絶対パスを壊してしまう。
func SanitiseHomePaths(text, home string) string {
	if home == "" {
		return text
	}
	cleaned := filepath.Clean(home)
	if filepath.Dir(cleaned) == cleaned {
		return text
	}
	var sanitised strings.Builder
	for index := 0; index < len(text); {
		if length, ok := homePathAt(text, index, cleaned); ok {
			sanitised.WriteByte('~')
			index += length
			continue
		}
		_, size := utf8.DecodeRuneInString(text[index:])
		sanitised.WriteString(text[index : index+size])
		index += size
	}
	return sanitised.String()
}

// homePathAt は、text の index の位置にホームがひとつのパスの先頭として現れて
// いるかを言い、そのホームの text でのバイト数を返す。
func homePathAt(text string, index int, home string) (int, bool) {
	if index > 0 {
		previous, _ := utf8.DecodeLastRuneInString(text[:index])
		if !outsidePath(previous) {
			return 0, false
		}
	}
	length, ok := nativepath.MatchPrefix(text[index:], home)
	if !ok {
		return 0, false
	}
	end := index + length
	if end == len(text) || os.IsPathSeparator(text[end]) {
		return length, true
	}
	next, _ := utf8.DecodeRuneInString(text[end:])
	return length, outsidePath(next)
}

// pathDelimiters は、パスの前後に来てもパスの一部とはみなさない記号である。
// エラー文の "open <パス>: ..."、引用符や括弧で囲んだパス、PATH のように
// ":" で区切った並び、"HOME=<パス>" の形を拾う。
const pathDelimiters = "\"'`()[]{}<>,;:="

func outsidePath(letter rune) bool {
	return unicode.IsSpace(letter) || strings.ContainsRune(pathDelimiters, letter)
}
