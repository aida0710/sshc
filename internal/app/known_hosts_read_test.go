package app

import (
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/knownhosts"
	"sshc/internal/storage"
)

// 接続ログの「known_hostsの何行目」は、known_hosts画面やエディタと同じ物理行で数える。
// 一致した行より前のコメント・空行・解析できない行を飛ばして数えると、別の行を指す。
func TestDialerReadsKnownHostsWithTheLineNumbersOfTheFile(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	contents := "# managed by hand\n" +
		"\n" +
		"not a known_hosts entry\n" +
		"example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGOsSkw6xSsvW0qbp1a1EVJLiuvJf2jn5ITuTaMlBdET\n"
	if err := os.WriteFile(filepath.Join(root, "known_hosts"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts := knownhosts.NewService(workspace, nil, knownhosts.Scanner{})

	read, err := readKnownHosts(hosts)()
	if err != nil {
		t.Fatal(err)
	}
	var numbers []int
	for _, line := range knownhosts.ParseFile(read).Entries() {
		numbers = append(numbers, line.Number)
	}
	if len(numbers) != 1 || numbers[0] != 4 {
		t.Fatalf("entry line numbers = %v, want [4] as in the file", numbers)
	}
}
