package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 経路の寿命（起動の途中の失敗、取り消し、無操作の停止、前回のコンテナの回収）を、
// 偽の docker で確かめる。本物の Docker を使う結合テスト（*_docker_test.go）は CI では
// 一部しか走らないので、分岐ごとの振る舞いはここで固定する。

// routeScript は、経路の起動を最後まで進める偽の docker の台本である。呼ばれた引数を
// calls に1行ずつ書く。この engine のコンテナはまだ無く、イメージは作成済みとして
// 答える。run と exec（設定を渡す）は、それぞれの台本で答える。
type routeScript struct {
	calls string
	run   string
	exec  string
}

func (script routeScript) String() string {
	return `echo "$@" >> '` + script.calls + `'
case "$1" in
container) echo 'Error response from daemon: No such container: x' >&2; exit 1 ;;
run) ` + script.run + ` ;;
exec) ` + script.exec + ` ;;
esac
exit 0
`
}

// dockerCallsFile は、偽の docker が呼ばれた引数を書く先を返す。
func dockerCallsFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "calls")
}

// dockerCalls は、偽の docker が呼ばれた引数を、呼ばれた順に返す。
func dockerCalls(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(contents), "\n"), "\n")
}

// requireRemoved は、コンテナを止めてから消したことを確かめる。
func requireRemoved(t *testing.T, calls []string, container string) {
	t.Helper()
	stopped := slices.IndexFunc(calls, func(call string) bool {
		return strings.HasPrefix(call, "stop ") && strings.HasSuffix(call, " "+container)
	})
	removed := slices.IndexFunc(calls, func(call string) bool {
		return strings.HasPrefix(call, "rm ") && strings.HasSuffix(call, " "+container)
	})
	if stopped < 0 || removed < stopped {
		t.Fatalf("コンテナ %s を止めて消していない: %q", container, calls)
	}
}

// 経路を起こせなかったら、起こす前に借りた分を返す。返さないと、その経路は誰も
// 使っていないのに、いつまでも無操作にならない。
func TestAFailedStartGivesBackWhatTheConnectionBorrowed(t *testing.T) {
	calls := dockerCallsFile(t)
	manager := managerWithDocker(t, routeScript{calls: calls, run: "echo 'docker: run failed' >&2; exit 1"}.String())
	profile := validProfile()

	_, err := manager.Dial(context.Background(), DialRequest{
		Profile: profile.Name, Source: fixedRoute(profile, validSecrets()),
		Address: "10.9.9.1:22",
	})

	if err == nil {
		t.Fatal("起こせない経路で繋がった")
	}
	if open := manager.state(profile.Name).openConnections(); open != 0 {
		t.Fatalf("借りたまま = %d", open)
	}
}

// 無操作と見てから鍵を取るまでのあいだに経路を使い始めた接続があれば、畳まない。
func TestAnIdleStopLeavesARouteThatWasBorrowedBeforeItTookTheLock(t *testing.T) {
	calls := dockerCallsFile(t)
	manager := managerWithDocker(t, routeScript{calls: calls}.String())
	started := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return started.Add(time.Hour) }
	state := manager.state("lab")
	state.markStarted(routeIdentity{}, nil, started)
	if err := state.transition.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		manager.StopIdle(context.Background(), time.Minute)
	}()
	waitFor(t, "StopIdle が無操作と見て鍵を待ち始める", func() bool { return state.transition.waiting.Load() == 1 })
	state.borrow()
	state.transition.unlock()
	select {
	case <-stopped:
	case <-time.After(promptly):
		t.Fatal("StopIdle が戻らない")
	}

	if !state.isRunning() {
		t.Fatal("使い始めた経路を畳んだ")
	}
	if called := dockerCalls(t, calls); len(called) != 0 {
		t.Fatalf("docker を呼んだ: %q", called)
	}
}

// トンネルまで上がっても engine の中継を開けなければ、コンテナを止めて消す。残すと、
// 誰も使えないコンテナが動き続ける。
func TestARouteWhoseRelayCannotOpenIsRemoved(t *testing.T) {
	calls := dockerCallsFile(t)
	manager := managerWithDocker(t, "")
	profile := validProfile()
	route := manager.routeDirectory(profile.Name)
	// 設定を渡したところでトンネルが上がったことにし、中継のソケットの場所には
	// 消せないディレクトリを置く。
	manager.docker = fakeDocker(t, routeScript{
		calls: calls,
		exec: "cat > /dev/null; : > '" + filepath.Join(route, statusFileName) + "'; " +
			"mkdir -p '" + filepath.Join(route, engineRelaySocketName, "blocker") + "'",
	}.String())

	err := manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))

	if err == nil {
		t.Fatal("中継を開けないのに起動できたことにした")
	}
	requireRemoved(t, dockerCalls(t, calls), manager.containerName(profile.Name))
	if manager.state(profile.Name).isRunning() {
		t.Fatal("中継の無い経路を動いていることにした")
	}
}

// 呼び出し側が起動を取り消しても、立てたコンテナは止めて消す。取り消された ctx の
// まま片付けると、docker を一度も呼べずにコンテナが残る。
func TestACanceledStartStillRemovesItsContainer(t *testing.T) {
	calls := dockerCallsFile(t)
	sending := filepath.Join(t.TempDir(), "sending")
	manager := managerWithDocker(t, routeScript{
		calls: calls, exec: "touch '" + sending + "'; exec sleep 30",
	}.String())
	profile := validProfile()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)

	go func() { done <- manager.Start(ctx, profile.Name, fixedRoute(profile, validSecrets())) }()
	waitFor(t, "起動が設定を渡すところまで進む", fileExists(sending))
	cancel()

	if err := receiveWithin(t, done, "Start"); err == nil {
		t.Fatal("取り消した起動が成功を返した")
	}
	requireRemoved(t, dockerCalls(t, calls), manager.containerName(profile.Name))
}

// 前回の engine のコンテナを回収すると予告したあいだは、経路を起こさない。起こした
// コンテナを、同じ engine の回収が止めてしまわないためである。
func TestAStartWaitsForTheDiscardOfOrphans(t *testing.T) {
	calls := dockerCallsFile(t)
	manager := managerWithDocker(t, routeScript{calls: calls, run: "exit 1"}.String())
	profile := validProfile()
	var read atomic.Bool
	source := func() (Profile, Secrets, error) {
		read.Store(true)
		return profile, validSecrets(), nil
	}
	manager.ExpectOrphanDiscard()
	done := make(chan error, 1)

	go func() { done <- manager.Start(context.Background(), profile.Name, source) }()
	time.Sleep(100 * time.Millisecond)
	if read.Load() || len(dockerCalls(t, calls)) != 0 {
		t.Fatalf("回収の前に起動を進めた: %q", dockerCalls(t, calls))
	}
	if err := manager.DiscardOrphans(context.Background()); err != nil {
		t.Fatalf("DiscardOrphans = %v", err)
	}

	_ = receiveWithin(t, done, "Start")
	called := dockerCalls(t, calls)
	if !read.Load() || len(called) < 2 || !strings.HasPrefix(called[0], "ps ") {
		t.Fatalf("回収のあとに起動していない: %q", called)
	}
}

// コンテナが動いたままトンネルが上がらなければ、待つ上限で打ち切って理由を返す。
func TestATunnelThatNeverComesUpTimesOut(t *testing.T) {
	// docker は、コンテナが動いていると答え続ける。
	manager := managerWithDocker(t, "echo true\n")
	clock := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	// 時計は読むたびに1分進む。上限（45秒）は、2回目に読んだときに過ぎている。
	manager.now = func() time.Time {
		clock = clock.Add(time.Minute)
		return clock
	}

	err := manager.waitForTunnel(context.Background(), manager.containerName("lab"), Profile{Name: "lab", Backend: WireGuard})

	var failure *RouteFailure
	if !errors.As(err, &failure) || failure.Reason != FailureTimeout {
		t.Fatalf("waitForTunnel = %v, want a timeout", err)
	}
}
