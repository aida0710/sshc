package terminal_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// tmuxProcess は、動いていた tmux のセッションに戻った Process である。
type tmuxProcess struct {
	*fakeProcess
	resumed bool
}

func (p *tmuxProcess) ResumedTmuxSession() bool { return p.resumed }

// tmuxNameSpy は、接続を開くときに渡された tmux のセッションの名前を集める。process があれば、
// 開くたびにそれが返す Process を返す。再接続は pump の goroutine から開くので mutex で守る。
type tmuxNameSpy struct {
	mutex   sync.Mutex
	names   []string
	process func(open int) terminal.Process
}

func (s *tmuxNameSpy) open(ctx context.Context, _ terminal.Size) (terminal.Process, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.names = append(s.names, terminal.TmuxSessionIn(ctx))
	if s.process != nil {
		return s.process(len(s.names)), nil
	}
	return newFakeProcess(), nil
}

func (s *tmuxNameSpy) seen() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.names...)
}

func TestEachSessionToAConnectionGetsItsOwnTmuxSession(t *testing.T) {
	spy := &tmuxNameSpy{}
	registry, _ := newFastRegistry()
	spec := terminal.Spec{Kind: terminal.KindSSH, Alias: "web.01:x", Title: "web", Open: spy.open}
	for range 3 {
		if _, err := registry.Open(context.Background(), spec); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"sshc-web_01_x", "sshc-web_01_x-2", "sshc-web_01_x-3"}
	if names := spy.seen(); !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestAClosedSessionGivesItsTmuxSessionNameBack(t *testing.T) {
	spy := &tmuxNameSpy{}
	registry, _ := newFastRegistry()
	spec := terminal.Spec{Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open}
	first, err := registry.Open(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(first.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Open(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if names := spy.seen(); !slices.Equal(names, []string{"sshc-web", "sshc-web"}) {
		t.Errorf("names = %v; opening the connection again does not return to its tmux session", names)
	}
}

func TestASessionOpenedAgainKeepsTheTmuxSessionItHad(t *testing.T) {
	spy := &tmuxNameSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", TmuxSession: "sshc-web-2", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if names := spy.seen(); names[0] != "sshc-web-2" || session.Reopening().TmuxSession != "sshc-web-2" {
		t.Errorf("names = %v, reopening = %+v", names, session.Reopening())
	}
}

func TestAReconnectOpensTheSameTmuxSession(t *testing.T) {
	first := newFakeProcess()
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		if open == 1 {
			return first
		}
		return newFakeProcess()
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return len(spy.seen()) == 2 })
	if names := spy.seen(); names[0] != names[1] {
		t.Errorf("names = %v", names)
	}
}

// 動いていた tmux のセッションに戻ったら、前のプログラムへコマンドを打ち込まない。Claude Code の入力欄に
// cd の行が入ってしまう。新しいシェルだとも言わない。
func TestReturningToARunningTmuxSessionTypesNothingIntoIt(t *testing.T) {
	first := newFakeProcess()
	second := &tmuxProcess{fakeProcess: newFakeProcess(), resumed: true}
	spy := &tmuxNameSpy{process: func(open int) terminal.Process {
		if open == 1 {
			return first
		}
		return second
	}}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open,
		Startup: func() terminal.Startup { return terminal.Startup{Commands: []string{"cd ~/src"}} },
	})
	if err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })
	first.exit(transportLost)
	testwait.Until(t, func() bool { return strings.Contains(string(snapshotOf(session)), "再接続しました") })

	second.mutex.Lock()
	typed := second.written.String()
	second.mutex.Unlock()
	if typed != "" {
		t.Errorf("typed %q into the program that was left running", typed)
	}
	if strings.Contains(string(snapshotOf(session)), "新しいシェル") {
		t.Errorf("the terminal says it is a new shell: %q", snapshotOf(session))
	}
}
