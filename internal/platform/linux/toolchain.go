//go:build linux

package linux

import "sshc/internal/platform/process"

// NewToolchain は、Linux の探索順を返す。
//
// PATH は意図的に参照しない。このアプリケーションが実行するプログラムが、継承した
// 環境に依存してはならないからだ。macOS 側と同じ理由であり、違うのは並びだけで
// ある。探し方そのものは process.Toolchain が持つ。
func NewToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/usr/local/bin", "/bin"},
	}
}
