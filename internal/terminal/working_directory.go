package terminal

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

// 作業フォルダは、セッションを開き直す場所である。エンジンの再起動はそこで開き直し、複製もそこで始まる。
// シェルは2通りのどちらかで報告する。OSC 7（`file://host/path`）は、macOS の zsh、fish、多くのプロンプトの
// テーマが送る。Debian と Ubuntu の bash は送らないが、既定のタイトルが `user@host: dir` なので、その
// タイトルも読む。

// MaxWorkingDirectoryBytes は、ターミナルの API が cwd に置く上限に合わせてある。
const MaxWorkingDirectoryBytes = 4096

// isDirectoryText は、シェルが報告したフォルダか API に送られたフォルダが、下の検査がさらに見てよい文字かを
// 返す。空でなく、MaxWorkingDirectoryBytes に収まり、制御文字と書式の文字を含まず、ネットワークやデバイスの
// パスでないことを確かめる。`//host/share` や `\\host\share` は Windows の UNC パスで、あるかを確かめる
// だけで、そのホストへ SMB でつなぎ、利用者の Windows の資格情報を差し出してしまう。値を決めるのはシェルの
// 出力なので、何かが開く前に断る。
func isDirectoryText(directory string) bool {
	if directory == "" || len(directory) > MaxWorkingDirectoryBytes {
		return false
	}
	if strings.HasPrefix(directory, "//") || strings.HasPrefix(directory, `\\`) {
		return false
	}
	return strings.IndexFunc(directory, isControlOrFormatCharacter) < 0
}

// isControlOrFormatCharacter は、C0、DEL、C1 の範囲 U+0080 から U+009F と、書式の文字を対象にする。C1 は、
// UTF-8 で書かれていても xterm.js が制御として読む（U+009B は CSI を始める）。双方向の上書きのような書式の
// 文字は、パスを実際と違うものに読ませる。フォルダは、シェルがそのまま映す `cd` の行でターミナルへ戻るので、
// どれも含んではならない。
func isControlOrFormatCharacter(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// parseOSC7Directory は、`OSC 7 ; file://host/path` のパスを読む。ホストは確かめない。SSH 越しでは、
// リモートのマシンの名前である。
func parseOSC7Directory(payload string) (string, bool) {
	location, err := url.Parse(payload)
	if err != nil || location.Scheme != "file" || location.User != nil || location.RawQuery != "" || location.Fragment != "" {
		return "", false
	}
	return checkedWorkingDirectory(location.Path)
}

// workingDirectoryFromTitle は、`user@host: dir` のタイトルのフォルダを読む。フォルダは絶対パスか、
// ホームフォルダの下（`~`、`~/src`）である。
func workingDirectoryFromTitle(title string) (string, bool) {
	// MaxTitle で切ったタイトルは、パスの途中で終わっているかもしれない。
	if utf8.RuneCountInString(title) >= MaxTitle {
		return "", false
	}
	identity, directory, found := strings.Cut(title, ": ")
	if !found || strings.ContainsAny(identity, " \t:") {
		return "", false
	}
	user, host, found := strings.Cut(identity, "@")
	if !found || user == "" || host == "" {
		return "", false
	}
	if directory != "~" && !strings.HasPrefix(directory, "~/") && !strings.HasPrefix(directory, "/") {
		return "", false
	}
	return checkedWorkingDirectory(directory)
}

func checkedWorkingDirectory(directory string) (string, bool) {
	if !isDirectoryText(directory) {
		return "", false
	}
	if directory[0] != '/' && directory[0] != '~' {
		return "", false
	}
	return directory, true
}

// acceptWorkingDirectory は、今の世代の process のシェルが報告したフォルダを記録する。再接続をまたいで
// 持ち続ける。次の process はホームフォルダで始まるが、開き直す価値があるのは、利用者が最後に作業していた
// 場所である。
func (s *Session) acceptWorkingDirectory(generation uint64, directory string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if generation != s.generation || s.exited != nil {
		return
	}
	s.noteWorkingDirectoryLocked(directory)
}

func (s *Session) noteWorkingDirectoryLocked(directory string) {
	if s.shell.startDirectory == "" {
		s.shell.startDirectory = directory
	}
	s.workingDirectory = directory
}

// WorkingDirectory は、セッションが今いるフォルダを返す。分からなければ ""。エンジンが調べられるローカルの
// process には、じかに尋ねる。ローカルのシェルの多くは何も報告しないためである。それ以外は、シェルが最後に
// 報告したフォルダを返す。SSH 越しでは `~` で始まることがある。
func (s *Session) WorkingDirectory() string {
	if directory, err := s.ForegroundDirectory(); err == nil {
		return directory
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.kind == KindShell {
		return localWorkingDirectory(s.workingDirectory)
	}
	return s.workingDirectory
}

// localWorkingDirectory は、報告されたフォルダを、このマシンで process を始められるフォルダに直す。直せなければ
// ""。OSC 7 は、Windows のドライブのパスを `/C:/Users` と書く。タイトルの `~` はシェルの書き方で、パスではない。
func localWorkingDirectory(reported string) string {
	if runtime.GOOS == "windows" && len(reported) >= 3 && reported[0] == '/' && reported[2] == ':' {
		reported = reported[1:]
	}
	local := filepath.FromSlash(reported)
	if !filepath.IsAbs(local) || !onLocalDrive(local) {
		return ""
	}
	return reported
}

// onLocalDrive は、絶対パスがこのマシンのドライブを指すかを返す。Windows ではドライブ文字である。UNC
// （`\\host\share`）やデバイス（`\\?\`、`\\.\`）のボリュームは、このマシンの外へ届く。ほかの OS に
// ボリュームは無い。
func onLocalDrive(path string) bool {
	if runtime.GOOS != "windows" {
		return true
	}
	volume := filepath.VolumeName(path)
	return len(volume) == 2 && volume[1] == ':'
}
