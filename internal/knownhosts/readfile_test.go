package knownhosts_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/knownhosts"
	"sshc/internal/storage"
)

// ワークスペースの外の known_hosts は読むだけである。OpenSSH と同じく、無いファイルと
// 通常のファイルでないもの（/dev/null）は空として扱い、シンボリックリンクはたどる。
// どれかで失敗すると、UserKnownHostsFile /dev/null を書いた接続先に繋がらない。
func TestReadFileOutsideTheWorkspaceTreatsWhatItCannotUseAsEmpty(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{os.DevNull, filepath.Join(t.TempDir(), "missing")} {
		contents, err := knownhosts.ReadFile(workspace, path)
		if err != nil || len(contents) != 0 {
			t.Errorf("ReadFile(%q) = %q, %v; want empty", path, contents, err)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}
	outside := t.TempDir()
	distributed := filepath.Join(outside, "ssh_known_hosts.real")
	if err := os.WriteFile(distributed, []byte("db.example "+fixtureKeyType+" "+fixtureKey+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(outside, "ssh_known_hosts")
	if err := os.Symlink(distributed, linked); err != nil {
		t.Fatal(err)
	}
	contents, err := knownhosts.ReadFile(workspace, linked)
	if err != nil || len(contents) == 0 {
		t.Errorf("ReadFile(symlink) = %q, %v; want the distributed file", contents, err)
	}
}

// 組織が配る GlobalKnownHostsFile は、設定ファイルの上限（1 MiB）より大きい。
// 読めないと、どの接続先でもホスト鍵を照合できず、接続が失敗する。
func TestReadFileOutsideTheWorkspaceReadsADistributedFileLargerThanAConfigFile(t *testing.T) {
	home := t.TempDir()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	line := "db.example " + fixtureKeyType + " " + fixtureKey + "\n"
	distributed := filepath.Join(t.TempDir(), "ssh_known_hosts")
	large := strings.Repeat(line, 2*storage.MaxFileSize/len(line))
	if err := os.WriteFile(distributed, []byte(large), 0o644); err != nil {
		t.Fatal(err)
	}
	contents, err := knownhosts.ReadFile(workspace, distributed)
	if err != nil || len(contents) != len(large) {
		t.Errorf("ReadFile = %d bytes, %v; want all %d bytes", len(contents), err, len(large))
	}
}

// ワークスペースの中の known_hosts も、照合のためには OpenSSH と同じく読む。
// シンボリックリンクはたどり、リンク先が無いものと通常のファイルでないものは空とする。
// 既定の ~/.ssh/known_hosts2 がリンクなだけで、どの接続先にも繋がらなくなっていた。
func TestReadFileInsideTheWorkspaceFollowsALinkLikeOpenSSH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link needs a privilege on Windows")
	}
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	line := "db.example " + fixtureKeyType + " " + fixtureKey + "\n"
	shared := filepath.Join(t.TempDir(), "known_hosts.shared")
	if err := os.WriteFile(shared, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{
		"known_hosts2": shared,
		"dangling":     filepath.Join(t.TempDir(), "missing"),
		"device":       os.DevNull,
	} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}

	if contents, err := knownhosts.ReadFile(workspace, filepath.Join(root, "known_hosts2")); err != nil || string(contents) != line {
		t.Errorf("ReadFile(known_hosts2) = %q, %v; want the linked file", contents, err)
	}
	for _, name := range []string{"dangling", "device"} {
		if contents, err := knownhosts.ReadFile(workspace, filepath.Join(root, name)); err != nil || len(contents) != 0 {
			t.Errorf("ReadFile(%s) = %q, %v; want empty", name, contents, err)
		}
	}
}
