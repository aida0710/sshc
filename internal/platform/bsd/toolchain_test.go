//go:build freebsd || openbsd

package bsd_test

import (
	"slices"
	"testing"

	"sshc/internal/platform/bsd"
)

// 探索順は固定であり、PATH は参照しない。ベースシステムの OpenSSH を先に見る。
func TestToolchainSearchesFixedDirectoriesInOrder(t *testing.T) {
	want := []string{"/usr/bin", "/usr/local/bin", "/bin"}
	if got := bsd.NewToolchain().Directories; !slices.Equal(got, want) {
		t.Fatalf("Directories = %#v, want %#v", got, want)
	}
}
