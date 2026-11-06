package terminal

import (
	"errors"

	"golang.org/x/sys/unix"
)

var errNoForegroundGroup = errors.New("terminal: the pseudo terminal has no foreground process group")

// foregroundGroup は、擬似端末の前面のプロセスグループの番号を返す。シェルが
// コマンドを動かしていないあいだは、シェル自身のグループである。
//
// fd は SyscallConn から借りる。File.Fd を呼ぶと file が blocking に戻り、Close が
// 読み込みを止められなくなる。
func (p *unixProcess) foregroundGroup() (int, error) {
	connection, err := p.file.SyscallConn()
	if err != nil {
		return 0, err
	}
	group, groupErr := 0, error(nil)
	if err := connection.Control(func(fd uintptr) {
		group, groupErr = unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	}); err != nil {
		return 0, err
	}
	if groupErr != nil {
		return 0, groupErr
	}
	if group <= 0 {
		return 0, errNoForegroundGroup
	}
	return group, nil
}
