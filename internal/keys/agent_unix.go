//go:build unix

package keys

import (
	"context"
	"net"

	"sshc/internal/platform"
)

// newPlatformAgent は、SSH_AUTH_SOCK の指す Unix ソケットへ接続する。
//
// 変数を読むのは OpenSSH 自身がそう約束しているからである。端末ごとに
// 別の agent が立つ環境があり、どれに通信するかを決めているのはその変数だけである。
func newPlatformAgent(lookup func(string) (string, bool)) platform.KeyAgent {
	return Agent{
		Socket: func() string {
			if lookup == nil {
				return ""
			}
			socket, _ := lookup("SSH_AUTH_SOCK")
			return socket
		},
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", address)
		},
	}
}
