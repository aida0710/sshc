package terminal_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// shellProcess は、シェル自身の出力を、sshc がターミナルへ書くものと分けて書ける、SSH の process である。
// sshclient.Session が渡すのと同じ形である（terminal.ShellOutput）。
type shellProcess struct {
	*readyProcess
	mutex    sync.Mutex
	watchers []func([]byte)
}

func (p *shellProcess) WatchShellOutput(seen func([]byte)) (stop func()) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.watchers = append(p.watchers, seen)
	index := len(p.watchers) - 1
	return func() {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		p.watchers[index] = nil
	}
}

// shell は、text をリモートのシェルの出力として書く。ターミナルと、シェルを見ているものの両方に届く。
func (p *shellProcess) shell(text string) {
	p.mutex.Lock()
	for _, seen := range p.watchers {
		if seen != nil {
			seen([]byte(text))
		}
	}
	p.mutex.Unlock()
	p.feed(text)
}

func (p *shellProcess) watching() bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	for _, seen := range p.watchers {
		if seen != nil {
			return true
		}
	}
	return false
}

type shellOpenSpy struct {
	mutex     sync.Mutex
	processes []*shellProcess
}

func (s *shellOpenSpy) open(context.Context, terminal.Size) (terminal.Process, error) {
	process := &shellProcess{readyProcess: newReadyProcess()}
	s.mutex.Lock()
	s.processes = append(s.processes, process)
	s.mutex.Unlock()
	return process, nil
}

func (s *shellOpenSpy) at(index int) *shellProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index >= len(s.processes) {
		return nil
	}
	return s.processes[index]
}

// openWithSteps は、起動時に steps を打ち込む SSH のセッションを開き、使える状態にする。各手順は、長くても
// wait だけ待つ。
func openWithSteps(t *testing.T, wait time.Duration, steps ...terminal.StartupStep) (*terminal.Session, *shellOpenSpy) {
	t.Helper()
	spy := &shellOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "router", Title: "router", Open: spy.open,
		Startup: func() terminal.Startup { return terminal.Startup{Steps: steps, Wait: wait} },
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).finishOpen(nil)
	t.Cleanup(func() {
		if process := spy.at(0); process != nil {
			process.exit(terminal.ExitInfo{Code: 0})
		}
	})
	return session, spy
}

func TestAStepWaitsForItsTextInTheShellOutputBeforeItTypes(t *testing.T) {
	_, spy := openWithSteps(t, 5*time.Second,
		terminal.StartupStep{Wait: "router>", Send: "enable", Enter: true},
		terminal.StartupStep{Wait: "Password:", Send: "s3cret", Enter: true},
		terminal.StartupStep{Wait: "router#", Send: "terminal length 0", Enter: true},
	)
	router := spy.at(0)
	testwait.Until(t, router.watching)

	router.shell("Welcome\r\n\x1b[1mrouter\x1b[0m")
	time.Sleep(20 * time.Millisecond)
	if got := router.keystrokes(); got != "" {
		t.Fatalf("typed %q before the prompt was complete", got)
	}
	// プロンプトは、色を除いた、ターミナルに出る形で照らし合わせる。
	router.shell(">")
	testwait.Until(t, func() bool { return router.keystrokes() == "enable\r" })

	router.shell("enable\r\nPassword: ")
	testwait.Until(t, func() bool { return router.keystrokes() == "enable\rs3cret\r" })
	router.shell("\r\nrouter#")
	testwait.Until(t, func() bool { return router.keystrokes() == "enable\rs3cret\rterminal length 0\r" })
}

func TestTextWrittenBeforeAStepWasTypedDoesNotAnswerTheNextStep(t *testing.T) {
	_, spy := openWithSteps(t, 5*time.Second,
		terminal.StartupStep{Wait: "$ ", Send: "sudo -i", Enter: true},
		terminal.StartupStep{Wait: "$ ", Send: "cd /srv", Enter: true},
	)
	shell := spy.at(0)
	testwait.Until(t, shell.watching)
	shell.shell("aida@web:~$ ")
	testwait.Until(t, func() bool { return shell.keystrokes() == "sudo -i\r" })
	time.Sleep(20 * time.Millisecond)
	if got := shell.keystrokes(); got != "sudo -i\r" {
		t.Fatalf("the prompt before sudo answered the next step too: %q", got)
	}
	shell.shell("\r\naida@web:~$ ")
	testwait.Until(t, func() bool { return shell.keystrokes() == "sudo -i\rcd /srv\r" })
}

func TestAStepWhoseTextDoesNotComeStopsTheStepsAndSaysWhy(t *testing.T) {
	session, spy := openWithSteps(t, 50*time.Millisecond,
		terminal.StartupStep{Wait: "Password:", Send: "s3cret", Enter: true},
		terminal.StartupStep{Send: "uptime", Enter: true},
	)
	shell := spy.at(0)
	testwait.Until(t, func() bool { return strings.Contains(string(snapshotOf(session)), "「Password:」") })
	if got := string(snapshotOf(session)); !strings.Contains(got, "\r\n[sshc] 接続時の自動実行を止めました。") {
		t.Fatalf("terminal = %q, want an [sshc] line of its own that says the steps stopped", got)
	}
	if got := shell.keystrokes(); got != "" {
		t.Fatalf("typed %q after the steps stopped", got)
	}
}

func TestAnOptionalStepWhoseTextDoesNotComeIsPassedOver(t *testing.T) {
	session, spy := openWithSteps(t, 50*time.Millisecond,
		terminal.StartupStep{Wait: "Last login", Send: "q", Optional: true},
		terminal.StartupStep{Send: "uptime", Enter: true},
	)
	shell := spy.at(0)
	testwait.Until(t, func() bool { return shell.keystrokes() == "uptime\r" })
	if got := string(snapshotOf(session)); strings.Contains(got, "[sshc]") {
		t.Fatalf("terminal = %q, want nothing said for an optional step", got)
	}
}

// sshc 自身がターミナルへ書くもの（認証の問い、[sshc] の行）は、シェルの出力ではない。手順がそれに答える
// ことは無い。
func TestAStepDoesNotAnswerWhatSshcWritesIntoTheTerminal(t *testing.T) {
	session, spy := openWithSteps(t, 100*time.Millisecond, terminal.StartupStep{Wait: "Password:", Send: "s3cret", Enter: true})
	shell := spy.at(0)
	testwait.Until(t, shell.watching)
	shell.feed("[sshc] Password: ")
	testwait.Until(t, func() bool {
		return strings.Contains(string(snapshotOf(session)), "接続時の自動実行を止めました")
	})
	if got := shell.keystrokes(); got != "" {
		t.Fatalf("typed %q into sshc's own prompt", got)
	}
}

func TestStepsStartAgainOnTheShellOfAReconnect(t *testing.T) {
	_, spy := openWithSteps(t, 5*time.Second, terminal.StartupStep{Wait: "$ ", Send: "cd /srv", Enter: true})
	first := spy.at(0)
	testwait.Until(t, first.watching)
	first.shell("$ ")
	testwait.Until(t, func() bool { return first.keystrokes() == "cd /srv\r" })
	first.exit(transportLost)

	testwait.Until(t, func() bool { return spy.at(1) != nil })
	second := spy.at(1)
	second.finishOpen(nil)
	testwait.Until(t, second.watching)
	second.shell("$ ")
	testwait.Until(t, func() bool { return second.keystrokes() == "cd /srv\r" })
	second.exit(terminal.ExitInfo{Code: 0})
}
