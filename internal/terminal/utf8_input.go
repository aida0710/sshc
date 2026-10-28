//go:build linux || darwin

package terminal

import (
	"os"

	"golang.org/x/sys/unix"
)

// enableUTF8Input は、端末の設定に IUTF8 を立てる。新しく作った PTY は Linux でも
// macOS でも切れていて、行単位で読むプログラム（cat、read、input()）の Backspace が
// 日本語を1バイトずつ消す。ブラウザのターミナルは UTF-8 しか送らない。
func enableUTF8Input(tty *os.File) error {
	fd := int(tty.Fd())
	attributes, err := unix.IoctlGetTermios(fd, getTermiosRequest)
	if err != nil {
		return err
	}
	attributes.Iflag |= unix.IUTF8
	return unix.IoctlSetTermios(fd, setTermiosRequest, attributes)
}
