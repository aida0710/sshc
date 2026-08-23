//go:build unix

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// canary は、このテストが探して回るための、ほかに現れようのない語である。
const canary = "correct-horse-battery-staple-9f2c1"

// 見つけたものを書き出さない。秘密を探す検査が、見つけた秘密を失敗の
// 文言に載せてしまえば、それを CI のログへ配ることになる。どこで見つけたかは
// 場所と経路だけで足りる。照合は指紋で行う。
func canaryDigest() string {
	sum := sha256.Sum256([]byte(canary))
	return hex.EncodeToString(sum[:])
}

// carriesCanary は、その断片に canary が含まれるかを、書き出さずに返す。
func carriesCanary(text string) bool {
	return strings.Contains(text, canary)
}

// liveHeadless は、隔離された家に headless の engine を一台立てる。
func liveHeadless(t *testing.T) (string, *testProcess) {
	t.Helper()
	home := isolatedHome(t)
	engine := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	return home, engine
}

// パスワードは端末からしか受け取らない。パイプの向こうから来たものは、
// 履歴にもスクリプトにも残りうる。判定できない問いを書き置いて先へ進むより、
// 断る方がよい。
func TestVaultPasswordCommandsRefuseRedirectedInput(t *testing.T) {
	home, _ := liveHeadless(t)

	for _, action := range []string{"create", "unlock", "change-password"} {
		process := start(t, home, "vault", action)
		// stdin は端末ではない。exec が既定で /dev/null を渡す。
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

// 打たれたパスワードは、どこにも書き残されない。引数にも、handoff にも、
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
		if carriesCanary(string(contents)) {
			found = append(found, strings.TrimPrefix(path, home))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		// 場所だけを言う。中身を載せれば、この検査そのものが漏らす経路になる。
		t.Errorf("the typed password (sha256 %s…) is readable in %v",
			canaryDigest()[:12], found)
	}
}

// 打たれたパスワードは、走っているプロセスからも見えない。argv は
// `ps` を打てる誰にでも読め、環境変数は子へ丸ごと継がれる。ディスクに
// 残さないことと、そこに出さないことは別の約束である。
func TestTheTypedPasswordIsNotVisibleOnAnyRunningProcess(t *testing.T) {
	home, engine := liveHeadless(t)

	create := startOnTerminal(t, home, "vault", "create")
	create.expect(t, "New master password: ", 20*time.Second)
	create.typeLine(t, canary)
	create.expect(t, "Confirm new master password: ", 20*time.Second)
	create.typeLine(t, canary)
	if code := create.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault create exit = %d", code)
	}

	for channel, text := range map[string]string{
		"the engine's command line": processCommandLine(t, engine.Command.Process.Pid),
		"the engine's environment":  processEnvironment(t, engine.Command.Process.Pid),
		"what the terminal echoed":  create.output.String(),
		"the engine's stdout":       engine.Stdout.String(),
		"the engine's stderr":       engine.Stderr.String(),
	} {
		if carriesCanary(text) {
			t.Errorf("the typed password (sha256 %s…) is visible in %s",
				canaryDigest()[:12], channel)
		}
	}
}

// vaultState は `sshc vault status` の一行を読む。engine に尋ねる。
// 保管庫のファイルを直接見ると、engine が知っている状態ではなく、ディスクに
// 残っている形を見ることになる。
func vaultState(t *testing.T, home string) string {
	t.Helper()
	process := start(t, home, "vault", "status")
	if code := process.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("vault status exit = %d\n%s", code, process.Stderr.String())
	}
	for _, line := range strings.Split(process.Stdout.String(), "\n") {
		// 列で読む。表はラベルを右詰めの空白で揃えるので、表記の前後を
		// そのまま切り出すと空白が付いてくる。
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "vault" {
			return fields[1]
		}
	}
	t.Fatalf("vault status printed no vault line:\n%s", process.Stdout.String())
	return ""
}
