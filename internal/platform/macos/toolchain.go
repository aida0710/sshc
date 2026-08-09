//go:build darwin

package macos

import "sshc/internal/platform/process"

// NewToolchain は、macOS の探索順を返す。
//
// /usr/bin が最初なのは、macOS に同梱される OpenSSH を設計上の対象にしている
// からである。Homebrew の prefix は、Apple のコピーが取り除かれたマシンのための
// フォールバックだ。探し方そのものは process.Toolchain が持つ。
func NewToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/opt/homebrew/bin", "/usr/local/bin"},
	}
}
