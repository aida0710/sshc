//go:build windows

package platform_test

import (
	"testing"

	"sshc/internal/platform"
)

// Windows のパスは大小文字と区切り文字を区別しない。ssh_config に違う綴りで
// 書いた IdentityFile も、同じホームとして伏せる。
func TestSanitiseHomePathsRewritesWindowsSpellingsOfTheSameHome(t *testing.T) {
	const home = `C:\Users\Name`
	for text, want := range map[string]string{
		`c:\users\name\.ssh\id: open c:\users\name\.ssh\id: Access is denied.`: `~\.ssh\id: open ~\.ssh\id: Access is denied.`,
		`C:/Users/Name/.ssh/id`:  `~/.ssh/id`,
		`C:\Users\Name2\.ssh\id`: `C:\Users\Name2\.ssh\id`,
	} {
		if got := platform.SanitiseHomePaths(text, home); got != want {
			t.Errorf("SanitiseHomePaths(%q) = %q, want %q", text, got, want)
		}
	}
}

// ドライブのルートは、どの絶対パスの先頭にも現れる。伏せる意味が無い。
func TestSanitiseHomePathsLeavesTextAloneWhenTheHomeIsADriveRoot(t *testing.T) {
	const text = `C:\foo C:\bar`

	if got := platform.SanitiseHomePaths(text, `C:\`); got != text {
		t.Errorf("SanitiseHomePaths with a drive root home = %q, want the text unchanged", got)
	}
}
