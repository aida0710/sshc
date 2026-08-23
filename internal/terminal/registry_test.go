package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
)

// fakeProcess は、実プロセスなしにレジストリの上を検査するための PTY である。
//
// 読み側は feed が押し込んだものを返し、exit が呼ばれるまで塞がる。これにより、
// 「終わっていない PTY」と「終わった PTY」をテストが正確に選べる。
type fakeProcess struct {
	onHangup func(*fakeProcess)
	mutex    sync.Mutex
	pending  [][]byte
	ready    chan struct{}
	done     chan struct{}
	info     terminal.ExitInfo

	written    bytes.Buffer
	sizes      []terminal.Size
	hangups    int
	closed     bool
	failResize error
}

func newFakeProcess() *fakeProcess {
	return &fakeProcess{ready: make(chan struct{}, 1), done: make(chan struct{})}
}

func (p *fakeProcess) feed(chunk string) {
	p.mutex.Lock()
	p.pending = append(p.pending, []byte(chunk))
	p.mutex.Unlock()
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

// exit は子プロセスの終了を演じる。Read は EOF を返し、Wait が info を返す。
func (p *fakeProcess) exit(info terminal.ExitInfo) {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return
	}
	p.closed = true
	p.info = info
	p.mutex.Unlock()
	close(p.done)
}

func (p *fakeProcess) Read(b []byte) (int, error) {
	for {
		p.mutex.Lock()
		if len(p.pending) > 0 {
			chunk := p.pending[0]
			p.pending = p.pending[1:]
			p.mutex.Unlock()
			return copy(b, chunk), nil
		}
		p.mutex.Unlock()
		select {
		case <-p.ready:
		case <-p.done:
			return 0, io.EOF
		}
	}
}

func (p *fakeProcess) Write(b []byte) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.written.Write(b)
}

func (p *fakeProcess) Resize(size terminal.Size) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.failResize != nil {
		return p.failResize
	}
	p.sizes = append(p.sizes, size)
	return nil
}

func (p *fakeProcess) Hangup() error {
	p.mutex.Lock()
	p.hangups++
	onHangup := p.onHangup
	p.mutex.Unlock()
	// **終わり方を差し替えられるようにする。** PTY を持つ相手は signal で
	// 終わるが、SSH のセッションは輸送ごと断たれて Code -1 で終わる
	// （sshclient.Session.Close）——そこを再現できないと、閉じたときの
	// 繋ぎ直しの判断が検査できない。
	if onHangup != nil {
		onHangup(p)
		return nil
	}
	p.exit(terminal.ExitInfo{Signal: "hangup"})
	return nil
}

func (p *fakeProcess) Wait() terminal.ExitInfo {
	<-p.done
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.info
}

func (p *fakeProcess) Close() error { return nil }

func (p *fakeProcess) hangupCount() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.hangups
}

func (p *fakeProcess) keystrokes() string {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.written.String()
}

func (p *fakeProcess) resizes() []terminal.Size {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return append([]terminal.Size(nil), p.sizes...)
}

// fakeStarter は、開かれた PTY をすべて手元に残す。
type fakeStarter struct {
	mutex     sync.Mutex
	processes []*fakeProcess
	commands  []terminal.Command
	err       error
}

func (s *fakeStarter) Start(_ context.Context, command terminal.Command, size terminal.Size) (terminal.Process, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	process := newFakeProcess()
	process.sizes = append(process.sizes, size)
	s.processes = append(s.processes, process)
	s.commands = append(s.commands, command)
	return process, nil
}

func (s *fakeStarter) last() *fakeProcess {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if len(s.processes) == 0 {
		return nil
	}
	return s.processes[len(s.processes)-1]
}

func (s *fakeStarter) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.processes)
}

func newRegistry(limits terminal.Limits) (*terminal.Registry, *fakeStarter) {
	starter := &fakeStarter{}
	return &terminal.Registry{
		Start:  starter,
		Limits: func() terminal.Limits { return limits },
	}, starter
}

func openShell(t testing.TB, registry *terminal.Registry) *terminal.Session {
	t.Helper()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "zsh", Size: terminal.Size{Cols: 80, Rows: 24},
		Command: terminal.Command{Path: "/bin/zsh"},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	return session
}

// waitFor は、pump が非同期に動くこの設計で「まだ届いていない」を待つ。
func waitFor(t testing.TB, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the condition never became true")
}

func TestOpenRefusesOnceTheLiveLimitIsReached(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 2, Scrollback: 1 << 10})

	first := openShell(t, registry)
	openShell(t, registry)

	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Command: terminal.Command{Path: "/bin/zsh"},
	}); !errors.Is(err, terminal.ErrSessionLimit) {
		t.Fatalf("the third Open() = %v, want ErrSessionLimit", err)
	}
	// 拒否は本当に拒否であって、開いてから閉じるのではない。
	if starter.count() != 2 {
		t.Fatalf("the refused request still started %d process(es)", starter.count()-2)
	}

	// 終了済みは生存上限に数えない。閉じた分だけ、また開けるようになる。
	if err := registry.Close(first.ID()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, exited(registry, first.ID()))
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Command: terminal.Command{Path: "/bin/zsh"},
	}); err != nil {
		t.Fatalf("Open() after one exited = %v", err)
	}
}

// **残す枠は「読まれていない終わり方」のためにある。**
//
// かつてここは registry.Close で閉じていたが、それは人が自分で閉じる操作であり、
// いまはその場で捨てられる——閉じた人はもう読んでいる。残るのは、自分から
// 終わったものだけである。
func TestExitedSessionsAreRetainedUpToTheCap(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})

	// 上限より多く開いて、**それぞれが自分から終わる。** 残るのは新しい方から
	// RetainedExited 本である。
	var opened []string
	for index := 0; index < terminal.RetainedExited+3; index++ {
		session := openShell(t, registry)
		opened = append(opened, session.ID())
		starter.processes[index].exit(terminal.ExitInfo{Code: 0})
		waitFor(t, exited(registry, session.ID()))
		registry.Prune()
	}

	sessions := registry.Sessions()
	if len(sessions) != terminal.RetainedExited {
		t.Fatalf("retained %d sessions, want %d", len(sessions), terminal.RetainedExited)
	}
	// 捨てられたのは古い方である。
	oldest := opened[len(opened)-terminal.RetainedExited]
	if sessions[0].ID != oldest {
		t.Fatalf("the oldest retained session is %q, want %q", sessions[0].ID, oldest)
	}
	for _, dropped := range opened[:len(opened)-terminal.RetainedExited] {
		if _, ok := registry.Lookup(dropped); ok {
			t.Fatalf("session %q should have been dropped", dropped)
		}
	}
}

func TestEverySessionGetsItsOwnIdentifier(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 20, Scrollback: 1 << 10})

	seen := map[string]bool{}
	for index := 0; index < 20; index++ {
		// 同じ alias に何本でも開ける。ID は alias ではない。
		session, err := registry.Open(context.Background(), terminal.Spec{
			Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
			Command: terminal.Command{Path: "/usr/bin/ssh"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if session.ID() == "" || seen[session.ID()] {
			t.Fatalf("identifier %q repeats", session.ID())
		}
		seen[session.ID()] = true
	}
}

func TestAttachReplaysTheBufferAndThenFollowsTheLiveOutput(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	process.feed("before-attach\n")
	waitFor(t, func() bool { return len(session.Snapshot()) > 0 })

	replay, stream := session.Attach()
	if string(replay) != "before-attach\n" {
		t.Fatalf("replay = %q", replay)
	}

	process.feed("after-attach\n")
	select {
	case chunk := <-stream.Output():
		if string(chunk) != "after-attach\n" {
			t.Fatalf("live chunk = %q", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the live output never arrived")
	}

	// 二度目のアタッチは、両方を再生する。
	again, _ := session.Attach()
	if string(again) != "before-attach\nafter-attach\n" {
		t.Fatalf("second replay = %q", again)
	}
}

func TestAnAttachmentThatDoesNotReadIsDroppedAndThePTYKeepsRunning(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	_, stalled := session.Attach()
	// 読まないまま、チャンネルの深さを超えて押し込む。
	for index := 0; index < 2000; index++ {
		process.feed("x")
	}
	waitFor(t, func() bool { return stalled.Dropped() })

	// 落とされたのはアタッチだけである。PTY は止まらず、バッファは進み続ける。
	if session.Exit() != nil {
		t.Fatal("the session died with its slow attachment")
	}
	process.feed("still-alive")
	waitFor(t, func() bool { return strings.Contains(string(session.Snapshot()), "still-alive") })

	// 繋ぎ直せる。同じセッションであり、落ちたのは通信路だけだった。
	_, fresh := session.Attach()
	process.feed("!")
	select {
	case <-fresh.Output():
	case <-time.After(2 * time.Second):
		t.Fatal("a fresh attachment received nothing")
	}
}

func TestExitLeavesTheSessionReadableAndClosesEveryAttachment(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	_, stream := session.Attach()
	// ssh が即座に失敗したときに読める唯一の場所がここである。
	process.feed("ssh: Could not resolve hostname\n")
	process.exit(terminal.ExitInfo{Code: 255})

	select {
	case <-stream.Output():
	case <-time.After(2 * time.Second):
		t.Fatal("the queued output never arrived")
	}
	waitFor(t, exited(registry, session.ID()))

	info := session.Exit()
	if info == nil || info.Code != 255 {
		t.Fatalf("Exit() = %+v, want code 255", info)
	}
	if stream.Dropped() {
		t.Fatal("a stream closed by the exit must not be reported as dropped")
	}
	// 終了済みでも出力は読める。理由が読める状態を保つのがこの保持の目的である。
	replay, closed := session.Attach()
	if !strings.Contains(string(replay), "Could not resolve hostname") {
		t.Fatalf("replay after exit = %q", replay)
	}
	select {
	case _, open := <-closed.Output():
		if open {
			t.Fatal("attaching to an exited session must not deliver live output")
		}
	case <-time.After(time.Second):
		t.Fatal("the stream for an exited session was left open")
	}

	if _, err := session.Write([]byte("x")); !errors.Is(err, terminal.ErrExited) {
		t.Fatalf("Write() after exit = %v, want ErrExited", err)
	}
	if err := session.Resize(terminal.Size{Cols: 10, Rows: 10}); !errors.Is(err, terminal.ErrExited) {
		t.Fatalf("Resize() after exit = %v, want ErrExited", err)
	}
}

func TestWriteAndResizeReachThePTY(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	if _, err := session.Write([]byte("echo hi\r")); err != nil {
		t.Fatal(err)
	}
	if got := process.keystrokes(); got != "echo hi\r" {
		t.Fatalf("keystrokes = %q", got)
	}

	if err := session.Resize(terminal.Size{Cols: 120, Rows: 34}); err != nil {
		t.Fatal(err)
	}
	sizes := process.resizes()
	if last := sizes[len(sizes)-1]; last.Cols != 120 || last.Rows != 34 {
		t.Fatalf("last size = %+v", last)
	}

	// 範囲の外は TIOCSWINSZ へ渡さない。
	for _, refused := range []terminal.Size{{Cols: 0, Rows: 24}, {Cols: 80, Rows: 0}, {Cols: 5000, Rows: 24}} {
		if err := session.Resize(refused); !errors.Is(err, terminal.ErrInvalidSize) {
			t.Fatalf("Resize(%+v) = %v, want ErrInvalidSize", refused, err)
		}
	}
}

// PTY を確保できない場合、セッションを作らずに理由を返す。何も起きていない
// セッションが一覧に並ぶより、開かなかったと言う方が正確である。
func TestAFailedStartCreatesNoSessionAndRunsTheCleanup(t *testing.T) {
	starter := &fakeStarter{err: errors.New("no pty available")}
	registry := &terminal.Registry{Start: starter, Limits: terminal.DefaultLimits}

	cleaned := false
	_, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Command: terminal.Command{Path: "/bin/zsh"},
		Cleanup: func() { cleaned = true },
	})
	if err == nil {
		t.Fatal("Open() accepted a starter that failed")
	}
	if len(registry.Sessions()) != 0 {
		t.Fatalf("a failed start left %d session(s)", len(registry.Sessions()))
	}
	if !cleaned {
		t.Fatal("a failed start did not run the cleanup")
	}
}

func TestCloseHangsUpTheLiveSessionAndForgetsTheExitedOne(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, exited(registry, session.ID()))
	if process.hangupCount() != 1 {
		t.Fatalf("hangups = %d, want 1", process.hangupCount())
	}
	// 一度目は SIGHUP。まだ一覧にいる。
	if _, ok := registry.Lookup(session.ID()); !ok {
		t.Fatal("the exited session left the list on the first close")
	}
	// 二度目が一覧から消す。
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup(session.ID()); ok {
		t.Fatal("the second close did not remove the session")
	}
	if err := registry.Close(session.ID()); !errors.Is(err, terminal.ErrNotFound) {
		t.Fatalf("Close() of a missing session = %v, want ErrNotFound", err)
	}
}

func TestTheSpecCommandReachesTheStarterUnchanged(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	command := terminal.Command{
		Path:      "/usr/bin/ssh",
		Arguments: []string{"--", "bastion"},
		Env:       []string{"HOME=/tmp"},
	}
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
		Command: command, Size: terminal.Size{Cols: 100, Rows: 40},
	}); err != nil {
		t.Fatal(err)
	}
	starter.mutex.Lock()
	defer starter.mutex.Unlock()
	got := starter.commands[0]
	if got.Path != command.Path || strings.Join(got.Arguments, " ") != "-- bastion" ||
		strings.Join(got.Env, " ") != "HOME=/tmp" {
		t.Fatalf("command = %+v", got)
	}
	if size := starter.processes[0].sizes[0]; size.Cols != 100 || size.Rows != 40 {
		t.Fatalf("initial size = %+v", size)
	}
}

func exited(registry *terminal.Registry, id string) func() bool {
	return func() bool {
		session, ok := registry.Lookup(id)
		return ok && session.Exit() != nil
	}
}

// 名前は表示だけのものである。走っているプロセスにも、ssh の相手にも、
// 識別子にも触れないことをここで固定する。触れば、同じ相手へ複数本開いた
// ときに行を見分けるという用途を超えてしまう。
func TestRenameChangesOnlyTheDisplayedName(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	before := starter.count()

	if err := registry.Rename(session.ID(), "  ログ監視  "); err != nil {
		t.Fatalf("Rename() = %v", err)
	}
	if got := session.Title(); got != "ログ監視" {
		t.Errorf("Title() = %q, want the trimmed name", got)
	}
	if got := session.View().Title; got != "ログ監視" {
		t.Errorf("View().Title = %q", got)
	}
	if session.Kind() != terminal.KindShell || !session.Live() {
		t.Errorf("rename disturbed the session: %#v", session.View())
	}
	if starter.count() != before {
		t.Error("rename started another process")
	}
}

// 終了した行も改名できる。読むために残してある行なので、印を付ける価値は
// そこにもある。
func TestRenameWorksOnAnExitedSession(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	session.Hangup()
	waitFor(t, func() bool { return !session.Live() })

	if err := registry.Rename(session.ID(), "落ちた方"); err != nil {
		t.Fatalf("Rename() on an exited session = %v", err)
	}
	if got := session.Title(); got != "落ちた方" {
		t.Errorf("Title() = %q", got)
	}
}

func TestRenameRefusesNamesTheListCannotShow(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)

	for _, refused := range []string{
		"",
		"   ",
		"esc\x1b[2J",
		"newline\nsecond",
		strings.Repeat("x", terminal.MaxTitle+1),
	} {
		if err := registry.Rename(session.ID(), refused); !errors.Is(err, terminal.ErrInvalidTitle) {
			t.Errorf("Rename(%q) = %v, want ErrInvalidTitle", refused, err)
		}
	}
	if got := session.Title(); got != "zsh" {
		t.Errorf("a refused rename changed the name to %q", got)
	}
	if err := registry.Rename("no-such-session", "name"); !errors.Is(err, terminal.ErrNotFound) {
		t.Errorf("Rename of an unknown session = %v, want ErrNotFound", err)
	}
}

// Spec.Open は、レジストリが PTY を知らないまま別のものを持てる継ぎ目である。
//
// プロセス内で話す SSH がここを通る。向こうにプロセスが無いので確保する PTY も
// 無く、Starter を通す理由がひとつも無い。この検査が Start を nil にしているのは、
// その経路が本当に Starter を必要としないことを言うためである。
func TestRegistryUsesTheSpecOwnOpenerWhenItHasOne(t *testing.T) {
	registry := &terminal.Registry{Limits: terminal.DefaultLimits}
	process := newFakeProcess()

	var asked terminal.Size
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
		Size: terminal.Size{Cols: 120, Rows: 40},
		Open: func(_ context.Context, size terminal.Size) (terminal.Process, error) {
			asked = size
			return process, nil
		},
	})
	if err != nil {
		t.Fatalf("Open() = %v", err)
	}
	if asked != (terminal.Size{Cols: 120, Rows: 40}) {
		t.Errorf("the opener was asked for %+v", asked)
	}
	if !session.Live() {
		t.Error("the session is not live")
	}
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
}

// 開けなかったときの扱いは Starter と同じでなければならない。片方だけが
// セッションを残すと、一覧に何も起きていない行が並ぶ。
func TestAFailedOpenerCreatesNoSessionAndRunsTheCleanup(t *testing.T) {
	registry := &terminal.Registry{Limits: terminal.DefaultLimits}

	cleaned := false
	_, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			return nil, errors.New("no route to host")
		},
		Cleanup: func() { cleaned = true },
	})
	if err == nil {
		t.Fatal("Open() accepted an opener that failed")
	}
	if len(registry.Sessions()) != 0 {
		t.Fatalf("a failed opener left %d session(s)", len(registry.Sessions()))
	}
	if !cleaned {
		t.Fatal("a failed opener did not run the cleanup")
	}
}

// 手段がひとつも無いレジストリは、開いたふりをしない。
func TestARegistryWithNeitherAStarterNorAnOpenerRefuses(t *testing.T) {
	registry := &terminal.Registry{Limits: terminal.DefaultLimits}
	if _, err := registry.Open(context.Background(), terminal.Spec{Kind: terminal.KindShell}); !errors.Is(err, terminal.ErrNoStarter) {
		t.Fatalf("Open() = %v, want ErrNoStarter", err)
	}
}
