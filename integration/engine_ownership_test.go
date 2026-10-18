package integration

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// 引数なしの `sshc` と `sshc open` は、engine が停止中なら裏で起動する。CLI は準備が
// できたら終わり、engine は残る。止めるのは `sshc stop` である。この性質はプロセスの
// 外からしか確かめられない。engine は CLI とは別のプロセスで動いている。
func TestOpenStartsAnEngineThatOutlivesTheCommandUntilStopped(t *testing.T) {
	home := isolatedHome(t)

	opened := start(t, home, "open", "--no-browser")
	if code := opened.wait(t, 40*time.Second); code != 0 {
		t.Fatalf("sshc open exited %d\n%s", code, opened.Stderr.String())
	}
	if !strings.Contains(opened.Stdout.String(), "#bootstrap=") {
		t.Errorf("stdout = %q, want a one-time UI URL", opened.Stdout.String())
	}
	if !strings.Contains(opened.Stderr.String(), "started the sshc engine in the background") {
		t.Errorf("stderr = %q, want it to say the engine was started", opened.Stderr.String())
	}
	if _, err := os.Stat(handoffPath(home)); err != nil {
		t.Fatalf("the background engine published no handoff after sshc open exited: %v", err)
	}

	reopened := start(t, home, "open", "--no-browser")
	if code := reopened.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("second sshc open exited %d\n%s", code, reopened.Stderr.String())
	}
	if strings.Contains(reopened.Stderr.String(), "started the sshc engine") {
		t.Errorf("second sshc open started another engine: %q", reopened.Stderr.String())
	}

	stopped := start(t, home, "stop")
	if code := stopped.wait(t, 40*time.Second); code != 0 {
		t.Fatalf("sshc stop exited %d\n%s", code, stopped.Stderr.String())
	}
	waitFor(t, 20*time.Second, "the stopped engine to remove its handoff", func() bool {
		_, err := os.Stat(handoffPath(home))
		return os.IsNotExist(err)
	})

	again := start(t, home, "stop")
	if code := again.wait(t, 20*time.Second); code != 0 {
		t.Fatalf("sshc stop without an engine exited %d\n%s", code, again.Stderr.String())
	}
	if !strings.Contains(again.Stdout.String(), "no sshc engine is running") {
		t.Errorf("stdout = %q, want it to say no engine is running", again.Stdout.String())
	}
}

// engine はアクセス URLを出さない。出せば、端末にもログにもワンタイムの資格情報
// が残る。読み手はスクロールバックであり、それは誰にでも見せられる場所ではない。
func TestHeadlessRunsInTheForegroundWithoutPublishingABootstrapToken(t *testing.T) {
	home := isolatedHome(t)

	process := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, process)

	if !process.running() {
		t.Fatalf("headless returned instead of holding the terminal\n%s", process.Stderr.String())
	}
	// handoff が出たことは、標準出力が書かれたことではない。どちらが先かは
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
	if document.Owner != handoff.OwnerEngine {
		t.Errorf("owner = %q, want headless", document.Owner)
	}
}

// 二台目に席は無い。engine はひとつであるという決めごとは、OS のロックが
// 守っている。
func TestASecondHeadlessRefusesToStart(t *testing.T) {
	home := isolatedHome(t)
	first := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, first)

	second := start(t, home, "engine")
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

// 勝つのは一台だけである。desktop と headless が同時に上がろうとしても、
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
				process = start(t, home, "engine")
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
		// 負けた側は、負けたと言って終わる。何かに躓いて落ちただけなら、
		// この検査は「一台しか残らなかった」を偶然で満たしてしまう。理由を端末に
		// 書いて 1 で終わるのが、ロックに弾かれたときの唯一の出方である。
		//
		// 以前はここが 3 も通していた。Electron のネイティブ層へ ownership の衝突を
		// 別番号で知らせていた頃の名残で、その読み手が消えた今は 1 しか出ない。
		code := process.wait(t, 5*time.Second)
		if code != 1 {
			t.Errorf("a loser exited with %d, want the refusal 1\n%s",
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

// 殺された engine は席を空ける。畳む機会は失われるが、OS のロックはその
// プロセスと一緒に消える。残った handoff は、次の owner が置き換える。
func TestAKilledEngineReleasesItsLockAndItsHandoffIsReplaced(t *testing.T) {
	home := isolatedHome(t)
	first := start(t, home, "engine")
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

	// takeOverAsHeadless が返った時点で、handoff は次の owner のものである。
	// 念のため、それが殺した方のものでないことを言っておく。
	next := takeOverAsHeadless(t, home)
	document := readHandoff(t, home)
	if document.PID == before.PID {
		t.Error("the stale handoff still names the engine that was killed")
	}
	if document.PID != next.Command.Process.Pid {
		t.Errorf("the handoff names pid %d, want the new owner %d",
			document.PID, next.Command.Process.Pid)
	}
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
