//go:build !windows

package windowspipe

import (
	"context"
	"errors"
	"net"
)

// AgentPipe は、Windows でだけ意味を持つ名前である。**他所では宛先ではない。**
// 定数を両方の側に置くのは、これを読む側が build tag を持たずに済むためで
// あり、その値がここで使えることを意味しない。
const AgentPipe = `\\.\pipe\openssh-ssh-agent`

// DialContext は、Windows 以外では常に断る。
//
// **代わりのものを推測しない。** ここに unix ソケットへの経路を置くと、
// 「named pipe へ繋いだ」と言いながら別のものへ繋ぐ関数になる。
func DialContext(context.Context, string) (net.Conn, error) {
	return nil, errors.New("named pipes exist only on Windows")
}
