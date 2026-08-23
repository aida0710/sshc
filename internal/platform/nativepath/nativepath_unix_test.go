//go:build !windows

package nativepath

import "testing"

func TestUnixTreatsDifferentCaseAsDifferentFiles(t *testing.T) {
	root := "/home/u/.ssh"
	if Contains(root, "/home/u/.SSH/config") {
		t.Fatal("a case-different path was reported inside the root")
	}
	if Identity(root+"/config") == Identity(root+"/CONFIG") {
		t.Fatal("case-different paths share an identity")
	}
}

// Unix では Windows の表記は絶対パスではない。ここで受け入れると、ワークスペース
// の外を指すパスが「絶対だから」という理由だけで通ってしまう。
func TestUnixRefusesWindowsSpellingsAsAbsolute(t *testing.T) {
	for _, path := range []string{`C:\Users\A\.ssh\config`, `C:/Users/A/.ssh/config`, `\\server\share\A\config`} {
		if Supported(path) {
			t.Errorf("Supported(%q) = true on a Unix filesystem", path)
		}
	}
	if !Supported("/home/u/.ssh/config") {
		t.Fatal("an ordinary Unix path was refused")
	}
}
