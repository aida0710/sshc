package macos_test

import (
	"slices"
	"testing"

	"sshc/internal/platform/macos"
)

func TestNewToolchainLooksAtTheSystemOpenSSHFirst(t *testing.T) {
	directories := macos.NewToolchain().Directories
	if len(directories) == 0 || directories[0] != "/usr/bin" {
		t.Fatalf("directories = %#v, want /usr/bin first", directories)
	}
}

// 探索順は固定であり、PATH は参照しない。このアプリケーションが実行する
// プログラムが、継承した環境に依存してはならない。
func TestToolchainSearchesFixedDirectoriesInOrder(t *testing.T) {
	want := []string{"/usr/bin", "/opt/homebrew/bin", "/usr/local/bin"}
	if got := macos.NewToolchain().Directories; !slices.Equal(got, want) {
		t.Fatalf("Directories = %#v, want %#v", got, want)
	}
}
