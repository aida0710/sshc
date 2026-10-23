package terminal

import (
	"errors"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// ForegroundDirectory は、擬似端末の前面のプロセスグループの作業ディレクトリを返す。
// シェルが Claude Code などを動かしているあいだは、そのプログラムのものになる。
//
// fd は SyscallConn から借りる。File.Fd を呼ぶと file が blocking に戻り、Close が
// 読み込みを止められなくなる。
func (p *unixProcess) ForegroundDirectory() (string, error) {
	connection, err := p.file.SyscallConn()
	if err != nil {
		return "", errors.Join(ErrDirectoryUnavailable, err)
	}
	group, groupErr := 0, error(nil)
	if err := connection.Control(func(fd uintptr) {
		group, groupErr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	}); err != nil {
		return "", errors.Join(ErrDirectoryUnavailable, err)
	}
	if groupErr != nil || group <= 0 {
		return "", errors.Join(ErrDirectoryUnavailable, groupErr)
	}
	// プロセスグループの番号は、その先頭のプロセスの番号でもある。先頭が先に
	// 終わったグループ（パイプラインの途中など）は読めず、分からないと答える。
	directory, err := os.Readlink("/proc/" + strconv.Itoa(group) + "/cwd")
	if err != nil {
		return "", errors.Join(ErrDirectoryUnavailable, err)
	}
	return directory, nil
}
