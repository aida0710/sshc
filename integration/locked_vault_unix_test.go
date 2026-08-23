//go:build unix

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// createLockedVault は、保管庫を作ってから施錠する。
func createLockedVault(t *testing.T, home string) {
	t.Helper()
	create := startOnTerminal(t, home, "vault", "create")
	create.expect(t, "New master password: ", 20*time.Second)
	create.typeLine(t, canary)
	create.expect(t, "Confirm new master password: ", 20*time.Second)
	create.typeLine(t, canary)
	if code := create.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault create exit = %d\n%s", code, create.output.String())
	}
	lock := start(t, home, "vault", "lock")
	if code := lock.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("vault lock exit = %d\n%s", code, lock.Stderr.String())
	}
}

// **施錠されていたら待たない。** かつては窓を前へ出して解錠を待っていたが、
// 前へ出す窓が無くなった。engine を動かしている人は端末に居るのだから、打つべき
// 語を渡して終わる方が短い。
func TestALockedVaultRefusesPromptlyWithTheVaultCommand(t *testing.T) {
	home, _ := liveHeadless(t)
	writeAliasConfig(t, home, "waiting-host")
	createLockedVault(t, home)

	connect := startOnTerminal(t, home, "waiting-host")

	if code := connect.wait(t, 20*time.Second); code != 1 {
		t.Errorf("exit = %d, want 1\n%s", code, connect.output.String())
	}
	if !strings.Contains(connect.output.String(), "sshc vault unlock") {
		t.Errorf("output = %q, want the vault command named", connect.output.String())
	}
}

// **engine が居なければ、起こし方と回り道を言う。** 起こすのはこのコマンドの
// 仕事ではない——生かしておくのは人である。
func TestConnectingWithoutAnEngineSaysHowToStartOne(t *testing.T) {
	home := t.TempDir()
	writeAliasConfig(t, home, "waiting-host")

	connect := startOnTerminal(t, home, "waiting-host")

	if code := connect.wait(t, 20*time.Second); code != 1 {
		t.Errorf("exit = %d, want 1\n%s", code, connect.output.String())
	}
	output := connect.output.String()
	for _, want := range []string{"sshc engine", "ssh"} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want it to name %q", output, want)
		}
	}
}

// writeAliasConfig は、繋ぎ先の名前をひとつだけ持つ設定を置く。
//
// 行き先は 127.0.0.1 の閉じたポートである。**接続が成功してはいけない**——
// ここで確かめるのは接続そのものではなく、接続へ至るまでの経路だからである。
func writeAliasConfig(t *testing.T, home, alias string) {
	t.Helper()
	directory := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	contents := "Host " + alias + "\n  HostName 127.0.0.1\n  Port 1\n  User nobody\n"
	if err := os.WriteFile(filepath.Join(directory, "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
