//go:build linux

package linux_test

import (
	"slices"
	"testing"

	"sshc/internal/platform/linux"
)

// 探索順は固定であり、PATH は参照しない。このアプリケーションが実行する
// プログラムが、継承した環境に依存してはならない。macOS 側と同じ理由である。
func TestToolchainSearchesFixedDirectoriesInOrder(t *testing.T) {
	want := []string{"/usr/bin", "/usr/local/bin", "/bin"}
	if got := linux.NewToolchain().Directories; !slices.Equal(got, want) {
		t.Fatalf("Directories = %#v, want %#v", got, want)
	}
}
