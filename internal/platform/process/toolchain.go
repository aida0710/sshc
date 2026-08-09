package process

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrProgramNotFound は、このアプリケーションがプログラムを実行してよいどの
// ディレクトリにも、その OpenSSH プログラムが入っていないことを報告する。
var ErrProgramNotFound = errors.New("OpenSSH program not found")

// Toolchain は、固定の絶対パスで OpenSSH のプログラムを見つける。
//
// PATH は意図的に参照しない。このアプリケーションが実行するプログラムが、継承した
// 環境に依存してはならないからだ。どのディレクトリをどの順で見るかはプラット
// フォームごとに違うが、探し方は同じである。だからここには一覧を持たない。
type Toolchain struct {
	Directories []string
	Stat        func(string) (fs.FileInfo, error)
}

// SSH は ssh クライアントの絶対パスを返す。
func (t Toolchain) SSH() (string, error) { return t.find("ssh") }

// KeyScan は ssh-keyscan の絶対パスを返す。
func (t Toolchain) KeyScan() (string, error) { return t.find("ssh-keyscan") }

// KeyGen は ssh-keygen の絶対パスを返す。
func (t Toolchain) KeyGen() (string, error) { return t.find("ssh-keygen") }

// KeyAdd は ssh-add の絶対パスを返す。
func (t Toolchain) KeyAdd() (string, error) { return t.find("ssh-add") }

func (t Toolchain) find(program string) (string, error) {
	stat := t.Stat
	if stat == nil {
		stat = os.Stat
	}
	for _, directory := range t.Directories {
		candidate := filepath.Join(directory, program)
		info, err := stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("%w: %s", ErrProgramNotFound, program)
}
