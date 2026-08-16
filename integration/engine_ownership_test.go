package integration

import (
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// **裸の `sshc` は engine を起こさない。** 誰が engine の寿命を持つかは外殻が
// 決めることで、端末で打たれた一語がそれを横取りしてはならない。この性質は
// プロセスの外からしか確かめられない——ロックはこのプロセスの中には無い。
func TestBareInvocationTakesNoEngineLock(t *testing.T) {
	if runtime.GOOS == "darwin" {
		// darwin の bare activation は LaunchServices に本物の束を訊く。
		// 開発機で走らせれば、その人が入れている sshc.app が、隔離した家では
		// なく本物の HOME に対して上がる。テストが利用者のアプリを起こしては
		// ならない。Linux は記録が無ければ断るので、そちらで確かめる。
		t.Skip("bare activation asks LaunchServices for a real bundle here")
	}
	home := isolatedHome(t)

	process := start(t, home)
	code := process.wait(t, 20*time.Second)

	if code == 0 {
		t.Errorf("exit = 0; nothing could have been activated in an empty home")
	}
	if _, err := os.Stat(lockPath(home)); !os.IsNotExist(err) {
		t.Errorf("bare sshc created %s; it must not own an engine", lockPath(home))
	}
	if _, err := os.Stat(handoffPath(home)); !os.IsNotExist(err) {
		t.Errorf("bare sshc published a handoff; it started an engine")
	}
	if !strings.Contains(process.Stderr.String(), "sshc headless") {
		t.Errorf("stderr = %q, want the headless command named", process.Stderr.String())
	}
}

// **headless は入口を出さない。** 出せば、端末にもログにもワンタイムの資格情報
// が残る。読み手はスクロールバックであり、それは誰にでも見せられる場所ではない。
func TestHeadlessRunsInTheForegroundWithoutPublishingABootstrapToken(t *testing.T) {
	home := isolatedHome(t)

	process := start(t, home, "headless")
	waitForFile(t, handoffPath(home), 30*time.Second, process)

	if !process.running() {
		t.Fatalf("headless returned instead of holding the terminal\n%s", process.Stderr.String())
	}
	// **handoff が出たことは、標準出力が書かれたことではない。** どちらが先かは
	// 約束されていないので、announcement そのものを待つ。
	waitFor(t, 20*time.Second, "the headless announcement", func() bool {
		return strings.Contains(process.Stdout.String(), "sshc vault")
	})
	announced := process.Stdout.String()
	if strings.Contains(announced, "#bootstrap=") {
		t.Errorf("the headless announcement carries a bootstrap token: %q", announced)
	}
	if strings.Contains(announced, "http://127.0.0.1:") {
		t.Errorf("the headless announcement carries an entrance URL: %q", announced)
	}
	if !strings.Contains(announced, "sshc vault") {
		t.Errorf("announcement = %q, want the next command named", announced)
	}

	document := readHandoff(t, home)
	if document.Owner != handoff.OwnerHeadless {
		t.Errorf("owner = %q, want headless", document.Owner)
	}
}

// 二台目に席は無い。**engine はひとつである**という決めごとは、OS のロックが
// 守っている。
func TestASecondHeadlessRefusesToStart(t *testing.T) {
	home := isolatedHome(t)
	first := start(t, home, "headless")
	waitForFile(t, handoffPath(home), 30*time.Second, first)

	second := start(t, home, "headless")
	code := second.wait(t, 20*time.Second)

	if code != 1 {
		t.Errorf("second headless exit = %d, want 1", code)
	}
	if !strings.Contains(second.Stderr.String(), "already running") {
		t.Errorf("stderr = %q, want the running engine named", second.Stderr.String())
	}
	if !first.running() {
		t.Error("the first engine died when the second one was refused")
	}
}

// **所有権のチャンネルが閉じることが、終わってよいという意味である。**
// 殺すのではない——閉じれば engine は端末も転送も vault も畳んでからロックを
// 外す。この経路は同じプロセスの中では作れない。
func TestClosingTheOwnershipChannelStopsTheEngine(t *testing.T) {
	home := isolatedHome(t)
	engine := startOwned(t, home)
	waitForFile(t, handoffPath(home), 30*time.Second, engine)

	if err := engine.Ownership.Close(); err != nil {
		t.Fatal(err)
	}

	if code := engine.wait(t, 20*time.Second); code != 0 {
		t.Errorf("exit = %d, want 0; closing the channel is an ordinary ending\n%s",
			code, engine.Stderr.String())
	}
	// 畳み終えたなら、次の owner のために場所は空いている。
	next := start(t, home, "headless")
	waitForFile(t, handoffPath(home), 30*time.Second, next)
	if readHandoff(t, home).Owner != handoff.OwnerHeadless {
		t.Error("the replacement engine did not take the seat the desktop engine left")
	}
}

// **勝つのは一台だけである。** desktop と headless が同時に上がろうとしても、
// 席はひとつしかない。
func TestOnlyOneOwnerWinsAStartRace(t *testing.T) {
	home := isolatedHome(t)
	const racers = 4

	processes := make([]*testProcess, 0, racers)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	for index := 0; index < racers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			var process *testProcess
			if index%2 == 0 {
				process = startOwned(t, home)
			} else {
				process = start(t, home, "headless")
			}
			mutex.Lock()
			processes = append(processes, process)
			mutex.Unlock()
		}(index)
	}
	wait.Wait()

	waitFor(t, 40*time.Second, "the race to settle", func() bool {
		alive := 0
		for _, process := range processes {
			if process.running() {
				alive++
			}
		}
		return alive == 1 && fileExists(handoffPath(home))
	})

	alive := 0
	for _, process := range processes {
		if process.running() {
			alive++
			continue
		}
		// **負けた側は、負けたと言って終わる。** 何かに躓いて落ちただけなら、
		// この検査は「一台しか残らなかった」を偶然で満たしてしまう。desktop は
		// engineBusyExit(3) を外殻へ返し、headless は端末に理由を書いて 1 で
		// 終わる——どちらの番号も、ロックに弾かれたことだけを意味する。
		code := process.wait(t, 5*time.Second)
		if code != 1 && code != 3 {
			t.Errorf("a loser exited with %d, want the refusal 1 or 3\n%s",
				code, process.Stderr.String())
		}
	}
	if alive != 1 {
		t.Fatalf("%d owners are running, want exactly 1", alive)
	}
	document := readHandoff(t, home)
	if document.PID == 0 {
		t.Error("the winner published no pid")
	}
}

// **殺された engine は席を空ける。** 畳む機会は失われるが、OS のロックはその
// プロセスと一緒に消える。残った handoff は、次の owner が置き換える。
func TestAKilledEngineReleasesItsLockAndItsHandoffIsReplaced(t *testing.T) {
	home := isolatedHome(t)
	first := start(t, home, "headless")
	waitForFile(t, handoffPath(home), 30*time.Second, first)
	before := readHandoff(t, home)

	if err := first.Command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	first.wait(t, 10*time.Second)

	// handoff は残ったままである。生きている証拠ではないことを、ここで見る。
	if !fileExists(handoffPath(home)) {
		t.Fatal("the killed engine's handoff is gone; this test no longer proves anything")
	}

	second := start(t, home, "headless")
	waitFor(t, 30*time.Second, "the next owner to replace the stale handoff", func() bool {
		if !second.running() {
			t.Fatalf("the next owner could not start\n%s", second.Stderr.String())
		}
		document, err := handoff.Read(stateDir(home))
		return err == nil && document.PID != before.PID
	})
}

func readHandoff(t *testing.T, home string) handoff.Handoff {
	t.Helper()
	document, err := handoff.Read(stateDir(home))
	if err != nil {
		t.Fatalf("read handoff: %v", err)
	}
	return document
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
