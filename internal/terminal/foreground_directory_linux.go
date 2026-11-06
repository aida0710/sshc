package terminal

import (
	"errors"
	"os"
	"strconv"
)

// ForegroundDirectory は、擬似端末の前面のプロセスグループの作業ディレクトリを返す。
// シェルが Claude Code などを動かしているあいだは、そのプログラムのものになる。
func (p *unixProcess) ForegroundDirectory() (string, error) {
	group, err := p.foregroundGroup()
	if err != nil {
		return "", errors.Join(ErrDirectoryUnavailable, err)
	}
	// プロセスグループの番号は、その先頭のプロセスの番号でもある。先頭が先に
	// 終わったグループ（パイプラインの途中など）は読めず、分からないと答える。
	directory, err := os.Readlink("/proc/" + strconv.Itoa(group) + "/cwd")
	if err != nil {
		return "", errors.Join(ErrDirectoryUnavailable, err)
	}
	return directory, nil
}
