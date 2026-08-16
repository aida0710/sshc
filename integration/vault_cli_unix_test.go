//go:build unix

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// canary は、このテストが探して回るための、ほかに現れようのない語である。
const canary = "correct-horse-battery-staple-9f2c1"

// liveHeadless は、隔離された家に headless の engine を一台立てる。
func liveHeadless(t *testing.T) (string, *testProcess) {
	t.Helper()
	home := isolatedHome(t)
	engine := start(t, home, "headless")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	return home, engine
}

// **パスワードは端末からしか受け取らない。** パイプの向こうから来たものは、
// 履歴にもスクリプトにも残りうる。答えられない問いを書き置いて先へ進むより、
// 断る方がよい。
func TestVaultPasswordCommandsRefuseRedirectedInput(t *testing.T) {
	home, _ := liveHeadless(t)

	for _, action := range []string{"create", "unlock", "change-password"} {
		process := start(t, home, "vault", action)
		// stdin は端末ではない——exec が既定で /dev/null を渡す。
		code := process.wait(t, 20*time.Second)

		if code == 0 {
			t.Errorf("vault %s accepted redirected input", action)
		}
		if action == "create" && !strings.Contains(process.Stderr.String(), "interactive terminal") {
			t.Errorf("vault %s stderr = %q, want the terminal requirement named",
				action, process.Stderr.String())
		}
	}
}

// create から lock、unlock までを、本物の端末の向こうで一巡させる。
func TestVaultCreateUnlockAndLockThroughATerminal(t *testing.T) {
	home, _ := liveHeadless(t)

	create := startOnTerminal(t, home, "vault", "create")
	create.expect(t, "New master password: ", 20*time.Second)
	create.typeLine(t, canary)
	create.expect(t, "Confirm new master password: ", 20*time.Second)
	create.typeLine(t, canary)
	if code := create.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault create exit = %d\n%s", code, create.output.String())
	}

	if state := vaultState(t, home); state != "unlocked" {
		t.Fatalf("vault is %q after create, want unlocked", state)
	}

	lock := start(t, home, "vault", "lock")
	if code := lock.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("vault lock exit = %d\n%s", code, lock.Stderr.String())
	}
	if state := vaultState(t, home); state != "locked" {
		t.Fatalf("vault is %q after lock, want locked", state)
	}

	unlock := startOnTerminal(t, home, "vault", "unlock")
	unlock.expect(t, "Master password: ", 20*time.Second)
	unlock.typeLine(t, canary)
	if code := unlock.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault unlock exit = %d\n%s", code, unlock.output.String())
	}
	if state := vaultState(t, home); state != "unlocked" {
		t.Fatalf("vault is %q after unlock, want unlocked", state)
	}
}

// **打たれたパスワードは、どこにも書き残されない。** 引数にも、handoff にも、
// state ディレクトリの中の何にも。端末に出ないことは create が echo を止めて
// いることで、それは別の場所（cmd/sshc の PTY テスト）が見ている。ここが見る
// のは、ディスクに残るものである。
func TestTheTypedPasswordIsNowhereOnDisk(t *testing.T) {
	home, _ := liveHeadless(t)

	create := startOnTerminal(t, home, "vault", "create")
	create.expect(t, "New master password: ", 20*time.Second)
	create.typeLine(t, canary)
	create.expect(t, "Confirm new master password: ", 20*time.Second)
	create.typeLine(t, canary)
	if code := create.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault create exit = %d\n%s", code, create.output.String())
	}

	// 端末に返ってきたものにも現れない。no-echo が効いていれば、打った文字は
	// 一度も戻ってこない。
	if strings.Contains(create.output.String(), canary) {
		t.Errorf("the password was echoed back to the terminal")
	}

	var found []string
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(contents), canary) {
			found = append(found, strings.TrimPrefix(path, home))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("the typed password is readable in %v", found)
	}
}

// vaultState は `sshc vault status` の一行を読む。**engine に尋ねる。**
// 保管庫のファイルを直接見ると、engine が知っている状態ではなく、ディスクに
// 残っている形を見ることになる。
func vaultState(t *testing.T, home string) string {
	t.Helper()
	process := start(t, home, "vault", "status")
	if code := process.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("vault status exit = %d\n%s", code, process.Stderr.String())
	}
	for _, line := range strings.Split(process.Stdout.String(), "\n") {
		if state, found := strings.CutPrefix(line, "vault: "); found {
			return strings.TrimSpace(state)
		}
	}
	t.Fatalf("vault status printed no vault line:\n%s", process.Stdout.String())
	return ""
}
