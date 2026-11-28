//go:build windows

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl"
)

// engine が置くものは、この利用者しか読めない。handoff にはワンタイムの
// 資格情報が入っており、engine.lock と保管庫は同じディレクトリに居る。Windows
// でそれを言うのは mode ビットではなく DACL であり、単体テストが確かめている
// のは windowsacl の書き方である。ここで見るのは、本物の engine が本当に
// その道を通ったかである。
func TestWhatALiveEngineWritesIsReadableOnlyByThisUser(t *testing.T) {
	home := isolatedHome(t)
	engine := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)

	for name, path := range map[string]string{
		"the state directory": stateDir(home),
		"the handoff":         handoffPath(home),
		"the engine lock":     lockPath(home),
	} {
		restricted, err := windowsacl.IsRestrictedToCurrentUser(path)
		if err != nil {
			t.Errorf("%s (%s): %v", name, path, err)
			continue
		}
		if !restricted {
			t.Errorf("%s (%s) is readable outside this user", name, path)
		}
	}
}

// 異常終了でもロックは解ける。LockFileEx はプロセスに紐づくので、
// 畳む機会を失っても OS がそれを外す。外れなければ、次の owner は永久に
// 上がれない。これは Windows でしか確かめられない。
func TestTheLockSurvivesNoOneAfterAnAbnormalDeath(t *testing.T) {
	home := isolatedHome(t)
	first := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, first)
	if _, err := os.Stat(lockPath(home)); err != nil {
		t.Fatalf("the engine holds no lock file: %v", err)
	}

	if err := first.Command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	first.wait(t, 20*time.Second)

	second := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, second)
	if !second.running() {
		t.Fatalf("the next owner could not take the lock\n%s", second.Stderr.String())
	}
}

// ConPTY の子孫が engine より長生きしないことは、ここでは確かめない。
// コンソールを開けるのは画面の側であり、そこへ届くにはアクセス URLが要る。engine は
// それを出力しないので、この検査からは届かない。実機で確かめる項目として
// docs/manual-test-matrix.md が持つ。

// lockPath は、engine がこの home で握るロックのファイルである。Windows の試験だけが使う。
func lockPath(home string) string {
	return filepath.Join(stateDir(home), "engine.lock")
}
