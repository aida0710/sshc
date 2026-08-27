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
	forces     int
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

func (p *fakeProcess) WriteExact(_ context.Context, b []byte) error {
	_, err := p.Write(b)
	return err
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
	if onHangup != nil {
		onHangup(p)
		return nil
	}
	p.exit(terminal.ExitInfo{Signal: "hangup"})
	return nil
}

func (p *fakeProcess) ForceClose() error {
	p.mutex.Lock()
	p.forces++
	p.mutex.Unlock()
	p.exit(terminal.ExitInfo{Signal: "killed"})
	return nil
}

func (p *fakeProcess) Wait() terminal.ExitInfo {
	<-p.done
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.info
}

func (p *fakeProcess) Close() error { return nil }

func (p *fakeProcess) forceCount() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.forces
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

func TestCommandWritesToThePreviewedSSHProcessWithoutOpeningAnother(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "production", Title: "production",
		Command: terminal.Command{Path: "/unused"},
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := registry.CommandTarget(session.ID())
	if err != nil {
		t.Fatal(err)
	}
	if target.Alias != "production" || target.Generation == 0 {
		t.Fatalf("target = %#v", target)
	}
	if err := registry.WriteCommand(context.Background(), target, "pwd"); err != nil {
		t.Fatal(err)
	}
	if got := starter.last().keystrokes(); got != "pwd\r" {
		t.Fatalf("process input = %q", got)
	}
	if starter.count() != 1 {
		t.Fatalf("broadcast opened %d extra process(es)", starter.count()-1)
	}
}

func TestCommandSupportsLocalAndRefusesChangedSessions(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	local := openShell(t, registry)
	localTarget, err := registry.CommandTarget(local.ID())
	if err != nil {
		t.Fatal(err)
	}
	if localTarget.Kind != terminal.KindShell || localTarget.Alias != "localhost" {
		t.Fatalf("local target = %#v", localTarget)
	}
	if err := registry.WriteCommand(context.Background(), localTarget, "pwd"); err != nil {
		t.Fatal(err)
	}
	if got := starter.last().keystrokes(); got != "pwd\r" {
		t.Fatalf("local process input = %q", got)
	}

	var processes []*fakeProcess
	var processesMutex sync.Mutex
	processAt := func(index int) *fakeProcess {
		processesMutex.Lock()
		defer processesMutex.Unlock()
		if index >= len(processes) {
			return nil
		}
		return processes[index]
	}
	processCount := func() int {
		processesMutex.Lock()
		defer processesMutex.Unlock()
		return len(processes)
	}
	registry.ReconnectDelay = func(int) time.Duration { return 0 }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "production", Title: "production",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			process := newFakeProcess()
			processesMutex.Lock()
			processes = append(processes, process)
			processesMutex.Unlock()
			return process, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	previewed, err := registry.CommandTarget(session.ID())
	if err != nil {
		t.Fatal(err)
	}
	processAt(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	waitFor(t, func() bool { return processCount() == 2 })
	waitFor(t, func() bool {
		current, targetErr := registry.CommandTarget(session.ID())
		return targetErr == nil && current.Generation != previewed.Generation
	})
	if err := registry.WriteCommand(context.Background(), previewed, "whoami"); !errors.Is(err, terminal.ErrGenerationChanged) {
		t.Fatalf("WriteCommand after reconnect = %v, want ErrGenerationChanged", err)
	}
	if got := processAt(1).keystrokes(); got != "" {
		t.Fatalf("replacement process received stale command %q", got)
	}
}

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
	if starter.count() != 2 {
		t.Fatalf("the refused request still started %d process(es)", starter.count()-2)
	}

	if err := registry.Close(first.ID()); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup(first.ID()); ok {
		t.Fatal("the force-stopped session still occupies the live limit")
	}
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Command: terminal.Command{Path: "/bin/zsh"},
	}); err != nil {
		t.Fatalf("Open() after one exited = %v", err)
	}
}

func TestExitedSessionsAreRetainedUpToTheCap(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})

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
	for index := 0; index < 2000; index++ {
		process.feed("x")
	}
	waitFor(t, func() bool { return stalled.Dropped() })

	if session.Exit() != nil {
		t.Fatal("the session died with its slow attachment")
	}
	process.feed("still-alive")
	waitFor(t, func() bool { return strings.Contains(string(session.Snapshot()), "still-alive") })

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

func TestExitedSSHSessionReconnectsWithTheSameIdentityAndScrollback(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 12})
	first := newFakeProcess()
	second := newFakeProcess()
	processes := []*fakeProcess{first, second}
	next := 0
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			process := processes[next]
			next++
			return process, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.feed("before exit\n")
	waitFor(t, func() bool { return strings.Contains(string(session.Snapshot()), "before exit") })
	first.exit(terminal.ExitInfo{Code: 255})
	waitFor(t, func() bool { return session.Exit() != nil })

	reconnected, err := registry.Reconnect(context.Background(), session.ID())
	if err != nil {
		t.Fatal(err)
	}
	if reconnected != session || reconnected.ID() != session.ID() {
		t.Fatal("Reconnect replaced the session identity")
	}
	view := session.View()
	if view.State != terminal.StateConnected || view.Exited != nil || view.Problem != "" {
		t.Fatalf("reconnected view = %#v", view)
	}
	if snapshot := string(session.Snapshot()); !strings.Contains(snapshot, "before exit") ||
		!strings.Contains(snapshot, "新しいシェル") {
		t.Fatalf("reconnected scrollback = %q", snapshot)
	}
	second.exit(terminal.ExitInfo{})
	waitFor(t, func() bool { return session.Exit() != nil })
}

func TestManualReconnectRefusesLiveAndLocalSessions(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	live := openShell(t, registry)
	if _, err := registry.Reconnect(context.Background(), live.ID()); !errors.Is(err, terminal.ErrReconnectUnavailable) {
		t.Fatalf("Reconnect(live) = %v", err)
	}
	starter.last().exit(terminal.ExitInfo{})
	waitFor(t, func() bool { return live.Exit() != nil })
	if _, err := registry.Reconnect(context.Background(), live.ID()); !errors.Is(err, terminal.ErrReconnectUnavailable) {
		t.Fatalf("Reconnect(local shell) = %v", err)
	}
}

func TestClosingWhileManualReconnectOpensDoesNotResurrectTheSession(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	first := newFakeProcess()
	replacement := newFakeProcess()
	entered := make(chan struct{})
	release := make(chan struct{})
	opened := 0
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			opened++
			if opened == 1 {
				return first, nil
			}
			close(entered)
			<-release
			return replacement, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.exit(terminal.ExitInfo{Code: 255})
	waitFor(t, func() bool { return session.Exit() != nil })

	result := make(chan error, 1)
	go func() {
		_, reconnectErr := registry.Reconnect(context.Background(), session.ID())
		result <- reconnectErr
	}()
	<-entered
	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	replacement.exit(terminal.ExitInfo{})
	close(release)
	if reconnectErr := <-result; !errors.Is(reconnectErr, terminal.ErrReconnectUnavailable) {
		t.Fatalf("Reconnect after close = %v", reconnectErr)
	}
	registry.Prune()
	if _, ok := registry.Lookup(session.ID()); ok {
		t.Fatal("the manually closed session was resurrected")
	}
}

func TestPendingManualReconnectCountsAsOneSession(t *testing.T) {
	registry, _ := newRegistry(terminal.Limits{MaxSessions: 2, Scrollback: 1 << 10})
	first := newFakeProcess()
	replacement := newFakeProcess()
	entered := make(chan struct{})
	release := make(chan struct{})
	opened := 0
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "bastion", Title: "bastion",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) {
			opened++
			if opened == 1 {
				return first, nil
			}
			close(entered)
			<-release
			return replacement, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.exit(terminal.ExitInfo{Code: 255})
	waitFor(t, func() bool { return session.Exit() != nil })

	result := make(chan error, 1)
	go func() {
		_, reconnectErr := registry.Reconnect(context.Background(), session.ID())
		result <- reconnectErr
	}()
	<-entered
	second := openShell(t, registry)
	if _, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Command: terminal.Command{Path: "/bin/zsh"},
	}); !errors.Is(err, terminal.ErrSessionLimit) {
		t.Fatalf("third Open() = %v, want ErrSessionLimit", err)
	}

	replacement.exit(terminal.ExitInfo{})
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := registry.Close(second.ID()); err != nil {
		t.Fatal(err)
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

	for _, refused := range []terminal.Size{{Cols: 0, Rows: 24}, {Cols: 80, Rows: 0}, {Cols: 5000, Rows: 24}} {
		if err := session.Resize(refused); !errors.Is(err, terminal.ErrInvalidSize) {
			t.Fatalf("Resize(%+v) = %v, want ErrInvalidSize", refused, err)
		}
	}
}

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

func TestCloseForceStopsAndImmediatelyForgetsTheLiveSession(t *testing.T) {
	registry, starter := newRegistry(terminal.Limits{MaxSessions: 4, Scrollback: 1 << 10})
	session := openShell(t, registry)
	process := starter.last()

	if err := registry.Close(session.ID()); err != nil {
		t.Fatal(err)
	}
	if process.forceCount() != 1 {
		t.Fatalf("forces = %d, want 1", process.forceCount())
	}
	if _, ok := registry.Lookup(session.ID()); ok {
		t.Fatal("the force-stopped session remained in the list")
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

func TestARegistryWithNeitherAStarterNorAnOpenerRefuses(t *testing.T) {
	registry := &terminal.Registry{Limits: terminal.DefaultLimits}
	if _, err := registry.Open(context.Background(), terminal.Spec{Kind: terminal.KindShell}); !errors.Is(err, terminal.ErrNoStarter) {
		t.Fatalf("Open() = %v, want ErrNoStarter", err)
	}
}
