package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/connectionlog"
)

// promptly は、検査の相手が詰まったときに、検査そのものを止めないための上限である。
const promptly = 5 * time.Second

// managerWithDocker は、script を docker とし、docker を見つけ終えた Manager を作る。
func managerWithDocker(t *testing.T, script string) *Manager {
	t.Helper()
	manager := New(shortSocketDirectory(t), 1000, nil)
	manager.docker, manager.found = fakeDocker(t, script), true
	return manager
}

// receiveWithin は、done から promptly までに受け取った値を返す。what は、戻らなかった
// ときに名指す呼び出しである。
func receiveWithin(t *testing.T, done <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(promptly):
		t.Fatalf("%s が戻らない", what)
		return nil
	}
}

// waitFor は、condition が成り立つまで少しずつ待つ。promptly までに成り立たなければ、
// what を名指して検査を止める。
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(promptly)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("%s のを待ったが、時間切れになった", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fileExists は、path にファイルができたかを確かめる条件である。偽の docker が、どこまで
// 進んだかを印のファイルで知らせるのに使う。
func fileExists(path string) func() bool {
	return func() bool {
		_, err := os.Stat(path)
		return err == nil
	}
}

// 起動が鍵を握っているあいだに、諦めた停止は待たずに戻り、docker を呼ばない。
func TestAStopWhoseCallerGaveUpDoesNotWaitForTheStart(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	manager := managerWithDocker(t, "echo \"$@\" >> '"+calls+"'\nexit 0\n")
	state := manager.state("lab")
	if err := state.transition.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer state.transition.unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)

	go func() { done <- manager.Stop(ctx, "lab") }()

	if err := receiveWithin(t, done, "Stop"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v", err)
	}
	if contents, _ := os.ReadFile(calls); len(contents) != 0 {
		t.Fatalf("docker を呼んだ: %q", contents)
	}
}

// 停止は、進んでいる起動（ここではイメージの確認で止まっている）を打ち切ってから止める。
// 打ち切られた起動は、失敗ではなく停止の理由で終わる。
func TestAStopCancelsTheStartInProgress(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "checking-image")
	manager := managerWithDocker(t, `case "$1" in
container) echo 'Error response from daemon: No such container: x' >&2; exit 1 ;;
image) touch '`+marker+`'; exec sleep 30 ;;
esac
exit 0
`)
	started := make(chan error, 1)
	go func() {
		started <- manager.Start(context.Background(), validProfile().Name, fixedRoute(validProfile(), validSecrets()))
	}()
	waitFor(t, "起動がイメージの確認まで進む", fileExists(marker))
	stopped := make(chan error, 1)

	go func() { stopped <- manager.Stop(context.Background(), validProfile().Name) }()

	if err := receiveWithin(t, stopped, "Stop"); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	if err := receiveWithin(t, started, "Start"); !errors.Is(err, ErrRouteStopped) {
		t.Fatalf("Start = %v, want ErrRouteStopped", err)
	}
}

// 鍵を待っているあいだに sshcエンジンが終わった起動も、停止の理由で終わる。
func TestClosingTheManagerEndsAStartWaitingForTheLockAsStopped(t *testing.T) {
	manager := managerWithDocker(t, "exit 0\n")
	profile := validProfile()
	state := manager.state(profile.Name)
	if err := state.transition.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer state.transition.unlock()
	started := make(chan error, 1)
	go func() {
		started <- manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))
	}()
	waitFor(t, "起動が鍵を待ち始める", func() bool { return state.transition.waiting.Load() == 1 })

	manager.Close()

	if err := receiveWithin(t, started, "Start"); !errors.Is(err, ErrRouteStopped) {
		t.Fatalf("Start = %v, want ErrRouteStopped", err)
	}
}

// buildingDocker は、コンテナもイメージも無く、イメージの作成で止まる docker である。呼ばれた
// 引数を calls へ1行ずつ書き、作成に入ったら marker を作る。
func buildingDocker(calls, marker string) string {
	return `echo "$@" >> '` + calls + `'
case "$1" in
container) echo 'Error response from daemon: No such container: x' >&2; exit 1 ;;
image) echo 'Error response from daemon: No such image: x' >&2; exit 1 ;;
build) touch '` + marker + `'; exec sleep 30 ;;
esac
exit 0
`
}

// イメージの作成の途中で利用者が切断した起動は、イメージの作成の失敗ではなく切断で終わり、
// 経路の記録にも失敗として残らない。起動を頼んだほかの入口（別のタブや sshc vpn up）に、
// 故障のように見せない。
func TestAStartCutShortWhileBuildingTheImageEndsWithTheDisconnect(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "building")
	manager := managerWithDocker(t, buildingDocker(filepath.Join(directory, "calls"), marker))
	profile := validProfile()
	started := make(chan error, 1)
	go func() {
		started <- manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))
	}()
	waitFor(t, "起動がイメージの作成まで進む", fileExists(marker))

	if err := manager.Disconnect(context.Background(), profile.Name); err != nil {
		t.Fatalf("Disconnect = %v", err)
	}

	err := receiveWithin(t, started, "Start")
	if !errors.Is(err, ErrRouteDisconnected) || errors.Is(err, ErrImageBuild) {
		t.Fatalf("Start = %v, want only ErrRouteDisconnected", err)
	}
	record := manager.state(profile.Name).record.text()
	if strings.Contains(record, "失敗") || !strings.Contains(record, "VPN経路の起動を中止しました") {
		t.Fatalf("経路の記録が打ち切りを失敗として書いた:\n%s", record)
	}
}

// sshcエンジンの終了（Close）で打ち切った起動も、イメージの作成の失敗ではなく停止で終わり、
// 経路の記録にも失敗として残らない。
func TestClosingTheManagerWhileBuildingTheImageEndsTheStartAsStopped(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "building")
	manager := managerWithDocker(t, buildingDocker(filepath.Join(directory, "calls"), marker))
	profile := validProfile()
	started := make(chan error, 1)
	go func() {
		started <- manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))
	}()
	waitFor(t, "起動がイメージの作成まで進む", fileExists(marker))

	manager.Close()

	err := receiveWithin(t, started, "Start")
	if !errors.Is(err, ErrRouteStopped) || errors.Is(err, ErrImageBuild) {
		t.Fatalf("Start = %v, want only ErrRouteStopped", err)
	}
	record := manager.state(profile.Name).record.text()
	if strings.Contains(record, "失敗") || !strings.Contains(record, "VPN経路の起動を中止しました") {
		t.Fatalf("経路の記録が打ち切りを失敗として書いた:\n%s", record)
	}
}

// 呼び出し側が取り消した起動は、docker を止めて打ち切った段でも、取り消しで終わる。
// docker を止めた失敗（signal: killed）だけを返すと、取り消したのか故障したのかを
// 呼び出し側が見分けられない。
func TestAStartCancelledByItsCallerWhileBuildingTheImageEndsWithTheCancellation(t *testing.T) {
	directory := t.TempDir()
	marker := filepath.Join(directory, "building")
	manager := managerWithDocker(t, buildingDocker(filepath.Join(directory, "calls"), marker))
	profile := validProfile()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan error, 1)
	go func() { started <- manager.Start(ctx, profile.Name, fixedRoute(profile, validSecrets())) }()
	waitFor(t, "起動がイメージの作成まで進む", fileExists(marker))

	cancel()

	if err := receiveWithin(t, started, "Start"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start = %v, want context.Canceled", err)
	}
}

// 停止は、鍵を待っている起動も打ち切る。同じプロファイルへ Terminal と SFTP が同時に接続
// した状態で切断しても、2本目がイメージの作成をやり直さない。
func TestAStopAlsoEndsTheStartWaitingBehindTheOneInProgress(t *testing.T) {
	directory := t.TempDir()
	calls, marker := filepath.Join(directory, "calls"), filepath.Join(directory, "building")
	manager := managerWithDocker(t, buildingDocker(calls, marker))
	profile := validProfile()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		first <- manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))
	}()
	waitFor(t, "1本目がイメージの作成まで進む", fileExists(marker))
	go func() {
		second <- manager.Start(context.Background(), profile.Name, fixedRoute(profile, validSecrets()))
	}()
	state := manager.state(profile.Name)
	waitFor(t, "2本目が鍵を待ち始める", func() bool { return state.transition.waiting.Load() == 1 })

	if err := manager.Disconnect(context.Background(), profile.Name); err != nil {
		t.Fatalf("Disconnect = %v", err)
	}

	for _, started := range []chan error{first, second} {
		if err := receiveWithin(t, started, "Start"); !errors.Is(err, ErrRouteDisconnected) {
			t.Fatalf("Start = %v, want ErrRouteDisconnected", err)
		}
	}
	contents, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if builds := strings.Count(string(contents), "build --tag"); builds != 1 {
		t.Fatalf("イメージの作成を %d 回始めた:\n%s", builds, contents)
	}
}

// 起動は、停止が鍵を返すまで設定を読まない。削除の書き込みと停止のあとで起動した
// 接続は、消えたプロファイルの設定では経路を起こさない。
func TestAStartReadsTheProfileOnlyAfterTheStopIsDone(t *testing.T) {
	calls := filepath.Join(t.TempDir(), "calls")
	manager := managerWithDocker(t, "echo \"$@\" >> '"+calls+"'\nexit 0\n")
	state := manager.state("lab")
	if err := state.transition.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	var read atomic.Bool
	gone := errors.New("the profile was removed")
	done := make(chan error, 1)

	go func() {
		done <- manager.Start(context.Background(), "lab", func() (Profile, Secrets, error) {
			read.Store(true)
			return Profile{}, Secrets{}, gone
		})
	}()
	waitFor(t, "起動が鍵を待ち始める", func() bool { return state.transition.waiting.Load() == 1 })
	if read.Load() {
		t.Fatal("停止が鍵を握っているあいだに設定を読んだ")
	}
	state.transition.unlock()

	if err := receiveWithin(t, done, "Start"); !errors.Is(err, gone) {
		t.Fatalf("Start = %v", err)
	}
	if contents, _ := os.ReadFile(calls); strings.Contains(string(contents), "run") {
		t.Fatalf("消えたプロファイルのコンテナを起動した: %q", contents)
	}
}

// docker を探しているあいだ、ほかの呼び出しは自分の ctx が終われば待つのをやめる。
func TestWaitingForTheDockerLookupFollowsTheCallersContext(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	if err := manager.lookup.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.lookup.unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)

	go func() { done <- manager.Available(ctx) }()

	if err := receiveWithin(t, done, "Available"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Available = %v", err)
	}
}

// プロファイルを削除したあとは、同じ名前の経路について、前の記録も用意できなかった
// ときのログも見せない。
func TestForgettingAProfileDropsItsRecordAndFailureLogs(t *testing.T) {
	manager := managerWithDocker(t, "echo 'Error response from daemon: No such container: x' >&2\nexit 1\n")
	manager.state("lab").record.Write(connectionlog.Detailed, "old-vpn.example.jp へ接続できませんでした")
	manager.state("lab").keepFailureLogs("olduser: authentication failed")

	if err := manager.Forget(context.Background(), "lab"); err != nil {
		t.Fatalf("Forget = %v", err)
	}

	logs, err := manager.Logs(context.Background(), "lab", Secrets{})
	if err != nil {
		t.Fatalf("Logs = %v", err)
	}
	if strings.Contains(logs, "old-vpn.example.jp") || strings.Contains(logs, "olduser") {
		t.Fatalf("前のプロファイルの記録が残った:\n%s", logs)
	}
}
