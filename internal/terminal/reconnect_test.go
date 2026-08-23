package terminal_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
)

// openSpy は、開き直しが何度呼ばれたかを数えながら Process を配る。
type openSpy struct {
	mutex     sync.Mutex
	processes []*fakeProcess
	calls     int
	failUpTo  int
}

func (s *openSpy) open(_ context.Context, _ terminal.Size) (terminal.Process, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.calls++
	// **最初の接続は落とさない。** 落とすと、そもそもセッションが開かない
	// ——ここで確かめたいのは「開いたあと落ち続けたらどうなるか」である。
	if s.calls > 1 && s.calls <= s.failUpTo {
		return nil, context.DeadlineExceeded
	}
	process := newFakeProcess()
	s.processes = append(s.processes, process)
	return process, nil
}

func (s *openSpy) at(index int) *fakeProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if index >= len(s.processes) {
		return nil
	}
	return s.processes[index]
}

func (s *openSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.calls
}

// **輸送が落ちたら繋ぎ直す。**
//
// 回線が切り替わった、蓋を閉じた、相手が再起動した——どれも人が終わらせた
// のではない。終了済みの行にして「もう一度開いてください」と言うのは、
// 打ち直させているのと同じである。
func TestALostTransportIsDialledAgain(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return spy.count() >= 2 }) // 繋ぎ直しに行かなかった
	if !session.Live() {
		t.Error("繋ぎ直しているあいだに、終了済みにされた")
	}

	// **繋がったことは画面に出る。** 黙って戻すと、前のシェルだと思って打ち続ける。
	// 繋ぎ直したことを言わなかった
	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直しました")
	})
	if !strings.Contains(string(session.Snapshot()), "新しいシェル") {
		t.Error("新しいシェルであることを言っていない: 前の続きだと思わせてはならない")
	}
}

// **シェルが終わったのなら、繋ぎ直さない。**
//
// 人が `exit` と打ったなら、開き直すのは頼まれていないことをすることである。
func TestAShellThatExitedIsLeftAlone(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).exit(terminal.ExitInfo{Code: 0})

	waitFor(t, func() bool { return !session.Live() }) // 終わらなかった
	if spy.count() != 1 {
		t.Errorf("繋ぎ直しに行った: 呼ばれた回数 %d", spy.count())
	}
}

// **落ち続ける相手は諦める。** 永遠に試み続けるより、そう言って終わる方がよい。
func TestGivingUpIsSaidOutLoud(t *testing.T) {
	spy := &openSpy{failUpTo: 1 + terminal.MaxReconnects}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 一度目は成功させ、そのあと落とす。
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() }) // 諦めなかった
	if !strings.Contains(string(session.Snapshot()), "諦めました") {
		t.Error("諦めたことを言っていない")
	}
}

// **ローカルのシェルには落ちる輸送が無い。** Spec.Open を持たないので、
// 繋ぎ直しの経路そのものに入らない。
func TestALocalShellIsNeverDialledAgain(t *testing.T) {
	registry, starter := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "zsh", Command: terminal.Command{Path: "/bin/sh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := len(starter.processes)

	starter.processes[len(starter.processes)-1].exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool { return !session.Live() }) // 終わらなかった
	if len(starter.processes) != before {
		t.Error("ローカルのシェルを繋ぎ直しに行った")
	}
}

// newFastRegistry は、繋ぎ直しを待たないレジストリである。
//
// **待つのは現実の時間だけで、確かめたい事実には要らない。** 既定の間隔では
// 5 回の再試行に 33 秒かかり、諦めるところを見るだけで試験が止まる。
func newFastRegistry() (*terminal.Registry, *fakeStarter) {
	registry, starter := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return 0 }
	return registry, starter
}

// **繋ぎ直しのあいだの打鍵は捨てる。溜めない。**
//
// 溜めれば、半分打ったコマンドが新しいシェルへ届く。打った人は前の続きの
// つもりで、届く先は別のシェルである。
func TestKeystrokesDuringAReconnectAreDropped(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newRegistry(terminal.DefaultLimits())
	// 待たせて、そのあいだに打つ。
	registry.ReconnectDelay = func(int) time.Duration { return 150 * time.Millisecond }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := spy.at(0)
	first.exit(terminal.ExitInfo{Code: terminal.TransportLost})

	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直します")
	})
	// **落ちない。** 打った人にエラーを見せる場面ではない。
	if _, err := session.Write([]byte("rm -rf /tmp/half")); err != nil {
		t.Fatalf("繋ぎ直しのあいだの打鍵が失敗した: %v", err)
	}

	waitFor(t, func() bool { return spy.count() >= 2 })
	waitFor(t, func() bool {
		return strings.Contains(string(session.Snapshot()), "繋ぎ直しました")
	})

	// 新しいシェルは、その行を受け取っていない。
	if got := spy.at(1).keystrokes(); got != "" {
		t.Errorf("溜めた打鍵が新しいシェルへ届いた: %q", got)
	}
}

// closingSpy は、閉じられたときに**輸送の断絶として**終わる相手を演じる。
//
// **実物がそうである。** sshclient.Session.Close は finish(ExitInfo{Code: -1}) を
// 呼ぶ——閉じる操作は輸送を断つので、あちらからは落ちたのと同じに見える。
// registry_test の fakeProcess は Code 0 で終わるので、この道は再現できない。
type closingSpy struct {
	mutex     sync.Mutex
	processes []*fakeProcess
}

func (s *closingSpy) open(_ context.Context, _ terminal.Size) (terminal.Process, error) {
	process := newFakeProcess()
	process.onHangup = func(p *fakeProcess) { p.exit(terminal.ExitInfo{Code: terminal.TransportLost}) }
	s.mutex.Lock()
	s.processes = append(s.processes, process)
	s.mutex.Unlock()
	return process, nil
}

func (s *closingSpy) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.processes)
}

// **自分で閉じた人に「繋ぎ直します」と言わない。**
//
// 閉じる操作は輸送を断つので、sshclient はそれを TransportLost として報告する
// ——落ちたのか閉じたのかは、あちらからは見分けられない。見分けられるのは
// こちら側で、閉じたことは stopping が知っている。
func TestClosingAConsoleDoesNotPromiseToDialAgain(t *testing.T) {
	spy := &closingSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return !session.Live() })
	if strings.Contains(string(session.Snapshot()), "繋ぎ直します") {
		t.Errorf("閉じた人に、起きない繋ぎ直しを予告した:\n%s", session.Snapshot())
	}
	if spy.count() != 1 {
		t.Errorf("開き直しに行った回数 = %d, want 1（閉じたのだから行かない）", spy.count())
	}
}

// **自分で閉じたコンソールは、一覧に残さない。**
//
// 終了済みを残すのは最後の出力を読ませるためであり、閉じた人はもう読んでいて、
// そのうえで閉じている。残せば、片付けたはずのものが並び続ける。
func TestAConsoleThePersonClosedLeavesTheList(t *testing.T) {
	spy := &closingSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}

	waitFor(t, func() bool { return len(registry.Sessions()) == 0 })
}

// **勝手に切れたものは残す。** そちらは読まれていない——なぜ切れたのかは、
// 残っている画面にしか書いていない。
func TestAConsoleThatDroppedStaysToBeRead(t *testing.T) {
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 繋ぎ直しを使い切らせる。**諦めたあとが、読まれるべき終わり方である。**
	for attempt := 0; attempt <= terminal.MaxReconnects; attempt++ {
		waitFor(t, func() bool { return spy.count() >= attempt+1 })
		spy.at(attempt).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	}

	waitFor(t, func() bool { return !session.Live() })
	if len(registry.Sessions()) != 1 {
		t.Errorf("sessions = %d, want the dropped console kept so its reason can be read", len(registry.Sessions()))
	}
}
