// Package windows は、Windows で実行してよいプログラムの在り処を決める。
//
// PATH を見ない。Windows の PATH には利用者が書き込めるディレクトリが並び、
// その並びを決めているのはこのアプリケーションではない。ここが返すのは、
// Windows 自身が置いた場所の絶対パスか、「無い」かのどちらかである。
//
// ここに Win32 の呼び出しは無い。%WINDIR% のような信頼の起点は呼び出し側が
// 渡す。その結果、この方針はどの OS の上でも検査できる。Windows でしか
// 走らせられない規則は、Windows でしか間違いに気付けない規則でもある。
package windows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrProgramNotFound は、Windows に同梱される OpenSSH がこのマシンに無いことを
// 報告する。
var ErrProgramNotFound = errors.New("OpenSSH program not found")

// Toolchain は、Windows に同梱された OpenSSH だけを信頼する。
//
// 探す場所は一つしかないので、Unix 側の process.Toolchain のような並びを持た
// ない。実行ビットも見ない。Windows の Perm() が運ぶのは所有者の書き込み
// ビットだけで、0o755 も 0o644 も同じ姿になる。そこで実行可否を決めれば、
// 何も実行できないか、何でも実行してよいことになる。
type Toolchain struct{ directory string }

// NewToolchain は、%WINDIR% の下の OpenSSH を指す Toolchain を返す。
//
// windowsDirectory を受け取るのは、環境変数を読む場所をこのパッケージに持ち
// 込まないためである。呼び出し側は Windows 自身に尋ねた値を渡す。
func NewToolchain(windowsDirectory string) Toolchain {
	if windowsDirectory == "" {
		return Toolchain{}
	}
	return Toolchain{directory: filepath.Join(windowsDirectory, "System32", "OpenSSH")}
}

// KeyGen は ssh-keygen.exe の絶対パスを返す。
func (t Toolchain) KeyGen() (string, error) {
	// 起点が無いなら、候補も無い。filepath.Join に空を渡せば相対パスが
	// でき、それを stat するのはこのプロセスの作業ディレクトリの中である。
	if t.directory != "" {
		candidate := filepath.Join(t.directory, keyGenProgram)
		if existingProgram(candidate) == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrProgramNotFound, keyGenProgram)
}

// keyGenProgram は Windows での表記である。拡張子を落とすと、同梱の OpenSSH が
// 無いマシンで、たまたま同じ名前の別のファイルが選ばれうる。
const keyGenProgram = "ssh-keygen.exe"

// existingProgram は、そこに実在する通常ファイルがあることだけを確かめる。
func existingProgram(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrInvalid
	}
	return nil
}
