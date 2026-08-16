//go:build unix

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDesktop は、Electron の代わりに engine を持つ。
//
// **本物の外殻は起こさない。** テストが開発者のインストールしたアプリを開けば、
// それは隔離した家ではなく本物の HOME に対して上がる。ここで要るのは
// 「desktop が持っている engine」という状態だけで、窓は要らない。
func fakeDesktop(t *testing.T, home string) *testProcess {
	t.Helper()
	engine := startOwned(t, home)
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	return engine
}

// createLockedVault は、保管庫を作ってから施錠する。desktop が施錠されている、
// という 8.2 の状態を作るためである。
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

// **待ちに上限は無い。** 解錠は人が行うもので、人が席を外す時間に上限は無い。
// 別の端末の `sshc vault unlock` は同じ engine を変えるので、待っていた CLI は
// そのまま先へ進む——打ち直させない。
func TestALockedDesktopMakesTheConnectionWaitUntilAnotherTerminalUnlocks(t *testing.T) {
	home := isolatedHome(t)
	writeAliasConfig(t, home, "waiting-host")
	fakeDesktop(t, home)
	createLockedVault(t, home)

	connect := startOnTerminal(t, home, "waiting-host")
	connect.expect(t, "the sshc vault is locked", 20*time.Second)

	// 施錠されているあいだ、CLI は生きたまま待つ。
	time.Sleep(time.Second)
	if !connect.running() {
		t.Fatalf("the connection gave up instead of waiting:\n%s", connect.output.String())
	}

	unlock := startOnTerminal(t, home, "vault", "unlock")
	unlock.expect(t, "Master password: ", 20*time.Second)
	unlock.typeLine(t, canary)
	if code := unlock.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault unlock exit = %d\n%s", code, unlock.output.String())
	}

	// 同じ CLI が、打ち直されることなく接続へ進む。行き先は居ないので接続その
	// ものは失敗するが、**失敗する理由が「施錠」から「繋がらない」へ移る**。
	//
	// 255 は OpenSSH が「繋がらなかった」に使う番号であり、sshc はそれに揃えて
	// いる。1 ではないのは、1 なら「sshc が断った」になってしまうからで、ここで
	// 起きたのは断りではなく、行き先が居なかったことである。
	if code := connect.wait(t, 30*time.Second); code != 255 {
		t.Errorf("exit = %d, want 255 from the refused connection\n%s", code, connect.output.String())
	}
	if strings.Count(connect.output.String(), "the sshc vault is locked") != 1 {
		t.Errorf("the wait announced the lock more than once:\n%s", connect.output.String())
	}
	// 施錠のせいで終わったのではないことを、理由の側から見る。
	if !strings.Contains(connect.output.String(), "127.0.0.1:1") {
		t.Errorf("the connection never reached its destination:\n%s", connect.output.String())
	}
}

// **待っている人は、いつでも降りられる。** Ctrl-C は失敗ではないので、失敗の
// 終了コードを返さない。130 は「信号で終わった」をシェルへ伝える番号である。
func TestInterruptingTheUnlockWaitExitsWith130(t *testing.T) {
	home := isolatedHome(t)
	writeAliasConfig(t, home, "waiting-host")
	fakeDesktop(t, home)
	createLockedVault(t, home)

	connect := startOnTerminal(t, home, "waiting-host")
	connect.expect(t, "the sshc vault is locked", 20*time.Second)
	connect.interrupt(t)

	if code := connect.wait(t, 20*time.Second); code != 130 {
		t.Errorf("exit = %d, want 130\n%s", code, connect.output.String())
	}
}

// **headless は待たない。** 見えない窓の前で待たせても、そこには誰も居ない。
// headless を動かしている人は端末に居るので、打つべき語を渡して終わる。
func TestALockedHeadlessRefusesPromptlyWithTheVaultCommand(t *testing.T) {
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
