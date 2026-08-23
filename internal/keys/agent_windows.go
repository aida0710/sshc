//go:build windows

package keys

import (
	"sshc/internal/platform"
	"sshc/internal/platform/windowspipe"
)

// newPlatformAgent は、Windows の OpenSSH エージェントの named pipe へ接続する。
//
// lookup を使わない。Unix の SSH_AUTH_SOCK は OpenSSH 自身の約束だが、
// Windows にその約束は無い。読める変数をひとつでも見れば、それは「鍵と
// パスフレーズを任意のパイプへ渡す方法」になる。PATH も見ない。宛先は
// 探索の結果ではなく、ひとつの決まった名前である。
func newPlatformAgent(func(string) (string, bool)) platform.KeyAgent {
	return Agent{
		Socket: func() string { return windowspipe.AgentPipe },
		Dial:   windowspipe.DialContext,
	}
}
