//go:build freebsd || openbsd

package bsd

import "sshc/internal/platform/process"

// NewToolchain は、FreeBSD と OpenBSD の探索順を返す。
//
// どちらも OpenSSH をベースシステムの /usr/bin に持ち、ports やパッケージで
// 入れたものは /usr/local/bin に置く。PATH を参照しない理由は Linux・macOS 側と
// 同じである。探し方そのものは process.Toolchain が持つ。
func NewToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/usr/local/bin", "/bin"},
	}
}
