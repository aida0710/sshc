package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"sshc/internal/connectionlog"
	"sshc/internal/platform"
)

// ErrRouteDisconnected は、利用者が切断した経路を、自動再接続では起動し直さない
// ことを表す。起動の途中で利用者が切断したときも、起動はこの理由で終わる。
var ErrRouteDisconnected = errors.New("the vpn route was disconnected by the user")

// ErrRouteStopped は、起動の途中で経路が停止されたことを表す。sshcエンジンの終了
// （Manager.Close）と、プロファイルの削除・名前の変更が、進んでいる起動と鍵を待っている
// 起動を打ち切る。
var ErrRouteStopped = errors.New("the vpn route was stopped while it was starting")

// Manager は、この engine が持つVPN経路の全体である。
//
// Dockerが無い機械でも作れる。使えるかどうかは、使う時点で確かめて理由を返す。
// engineの起動が、入っていないかもしれないものに依存しないためである。
type Manager struct {
	// directory は、プロファイルごとの経路の置き場所（routeDirectory）を置く場所である。
	directory string
	// owner は、このengineを動かしている利用者である。コンテナの名前と札に使う。
	owner int
	// workspace は、この engine の workspace を表す短い識別子である。同じ利用者の
	// 別の workspace の engine が立てたコンテナと取り違えないために使う。
	workspace string

	// environment は、docker を探して起動する環境を返す。nil なら engine の環境を使う。
	environment Environment
	// imageName は、経路に使うイメージの名前（タグを除く）である。既定は
	// defaultImageName で、VPN の結合テストだけが別の名前にする。同じ名前のほかの
	// イメージは、イメージを作るたびに消す（removeOtherImages）ので、テストが
	// インストールした sshc のイメージを消さないように名前を分ける。
	imageName string

	// now は、無操作の長さを測る時計である。検査が差し替える。
	now func() time.Time

	// lifetime は、この Manager の寿命である。Close で終わり、進行中の起動を
	// 打ち切る。
	lifetime context.Context
	close    context.CancelFunc

	// orphans は、前回の engine が残したコンテナの回収が終わると閉じる。
	// 経路を起こす前にこれを待つ。起動直後に立てたコンテナを、同じ engine の
	// 回収が止めてしまわないためである。
	orphans *orphanGate

	// lookup は、docker を探すのを1本にする。ログインシェルの起動と docker info を
	// 待つあいだも、経路の状態は mutex で読める。待つ側は ctx が終われば待つのをやめる。
	lookup contextLock
	docker dockerCommand
	found  bool
	// variables は、一度読んだ docker を起動する環境である。画面は VPN の一覧を
	// 読み直し続けるので、Docker が無いあいだも毎回シェルを起動しない。
	// loaded は、読み終えたことを表す（読めずに engine の環境を使う場合も含む）。
	variables []string
	loaded    bool
	// pathSource は、docker を探した PATH をどこから取ったかの説明である。
	pathSource string
	// lookupFailure は、最後に docker を探して使えなかった理由と、その時刻である。
	// 経路の状態を読むときは、routeReadingLifetime のあいだ探し直さずにこれを使う。
	lookupFailure  error
	lookupFailedAt time.Time

	mutex  sync.Mutex
	routes map[string]*routeState

	// readings は、docker から読んだ経路の状態を短い時間だけ覚える。
	readings routeReadings
	// readContainers は、コンテナがあるプロファイルと、そのコンテナが動いているかを
	// 読む。既定は listContainers（docker ps）で、検査が差し替える。
	readContainers func(context.Context) (map[string]bool, error)
	// changes は、sshcエンジンが経路を起動・停止するたびに増える番号である。
	// 覚えた状態と、その後の起動・停止のどちらが新しいかを見分ける。
	changes atomic.Uint64
}

// New は、VPN経路の管理を作る。
//
// directory は、engineだけが読み書きするディレクトリの下を渡す。workspace の
// 識別子は、この directory から決まる。environment は、docker を探して起動する
// 環境を返す（nil なら engine の環境）。
func New(directory string, owner int, environment Environment) *Manager {
	lifetime, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		directory: directory, owner: owner, workspace: workspaceIdentity(directory),
		environment: environment, imageName: defaultImageName, now: time.Now,
		lifetime: lifetime, close: cancel, orphans: newOrphanGate(),
		routes: map[string]*routeState{},
	}
	manager.readContainers = manager.listContainers
	return manager
}

// Close は、進行中の起動を打ち切る。経路そのものは StopAll で畳む。
func (manager *Manager) Close() { manager.close() }

// Available は、このマシンでVPN経路を使えるかを返す。
//
// ここで見るのは Docker だけである。トンネルのデバイスは backend ごとに違うので、
// そのプロファイルを起こすときに確かめる。使えない理由は隠さない。利用者が何を
// 用意すればよいかを決める情報である。
func (manager *Manager) Available(ctx context.Context) error {
	_, err := manager.command(ctx)
	return err
}

// RouteSource は、プロファイルひとつぶんの保存済みの設定とシークレットを読む。
// プロファイルが無くなっていれば、その理由を返す。
//
// 経路の起動は、起動と停止の鍵を取ったあとでこれを呼び、読み直す。削除や改名の書き込みの
// 前に設定を読んだ接続が、そのあとの停止より遅れて、消えたプロファイルの経路を起こし
// 直さないためである。
type RouteSource func() (Profile, Secrets, error)

// DialRequest は、Dial がどのプロファイルの経路で、どこへ接続するかである。
type DialRequest struct {
	// Profile は、経路にするプロファイルの名前である。
	Profile string
	// Source は、そのプロファイルの設定とシークレットを読む。起動は鍵を取ってから読み直す。
	Source RouteSource
	// Address は、このプロファイルを付けた接続の HostName と Port（`host:port`）である。
	Address string
}

// Dial は、request.Profile の経路を通して、request.Address へのTCP接続を返す。
//
// 必要ならコンテナを起こし、経路ができるまで待つ。返るのはコンテナの中の中継の
// 標準入出力であり、その先のTCPはコンテナのトンネルを通る。
func (manager *Manager) Dial(ctx context.Context, request DialRequest) (net.Conn, error) {
	if err := validateProfileName(request.Profile); err != nil {
		return nil, err
	}
	ctx = manager.recording(ctx, request.Profile)
	// 接続先は、コンテナを起こす前に確かめる。起こしてから断るより早い。
	profile, _, err := request.Source()
	if err != nil {
		return nil, err
	}
	connectionlog.Say(ctx, connectionlog.Detailed, "%sへ、VPNプロファイル「%s」（%s）の経路で接続します。",
		request.Address, profile.Name, profile.Backend)
	destination, err := profile.Destination(request.Address)
	if err != nil {
		connectionlog.Say(ctx, connectionlog.Detailed, "接続先をVPN経由で使用できません：%v", err)
		return nil, err
	}
	// 起動より先に借りる。起動が終わってから借りるまでのあいだに、無操作と
	// 見なされて停止されないためである。
	state := manager.state(request.Profile)
	state.borrow()
	if err := manager.Start(ctx, request.Profile, request.Source); err != nil {
		state.release(manager.now())
		return nil, err
	}
	return manager.connectCounted(ctx, state, request.Profile, destination)
}

// dialRelay は、engine の中継が受けた接続のために、起動済みの経路で接続先へ繋ぐ。
func (manager *Manager) dialRelay(ctx context.Context, profile Profile, address string) (net.Conn, error) {
	ctx = manager.recording(ctx, profile.Name)
	connectionlog.Say(ctx, connectionlog.Detailed, "sshcエンジンの中継が、%sへの接続を受け付けました。", address)
	destination, err := profile.Destination(address)
	if err != nil {
		connectionlog.Say(ctx, connectionlog.Detailed, "接続先をVPN経由で使用できません：%v", err)
		return nil, err
	}
	state := manager.state(profile.Name)
	state.borrow()
	return manager.connectCounted(ctx, state, profile.Name, destination)
}

// connectCounted は、借りを済ませた経路で接続先へ繋ぎ、閉じたときに借りを返す
// 接続にする。繋げなければ借りを返す。
func (manager *Manager) connectCounted(
	ctx context.Context, state *routeState, profileName string, destination Endpoint,
) (net.Conn, error) {
	started := time.Now()
	connection, err := manager.connectTarget(ctx, profileName, destination)
	elapsed := connectionlog.Elapsed(time.Since(started))
	if err != nil {
		connectionlog.Say(ctx, connectionlog.Detailed, "VPN経由で%sに接続できませんでした（%s）：%v",
			destination.Address(), elapsed, err)
		state.release(manager.now())
		return nil, err
	}
	connectionlog.Say(ctx, connectionlog.Brief, "VPN経由で%sに接続しました（%s）。", destination.Address(), elapsed)
	return &countedConnection{Conn: connection, release: func() { state.release(manager.now()) }}, nil
}

// StopIdle は、接続が一本も通っていない状態が idle を超えた経路を停止する。
//
// 停止しないままにすると、一度使った経路のコンテナが engine の寿命のあいだ
// 残り続ける。トンネルは使っているあいだだけあればよい。
func (manager *Manager) StopIdle(ctx context.Context, idle time.Duration) {
	for _, name := range manager.names() {
		if manager.state(name).idleLongerThan(manager.now(), idle) {
			_ = manager.stopIfIdle(ctx, name, idle)
		}
	}
}

// stopIfIdle は、起動と停止の鍵を取ってから無操作かを確かめ直し、そうなら停止する。
//
// 確かめてから鍵を取るまでのあいだに、経路を使い始めた接続があるかもしれない。
func (manager *Manager) stopIfIdle(ctx context.Context, name string, idle time.Duration) error {
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	state := manager.state(name)
	if err := state.transition.lock(ctx); err != nil {
		return err
	}
	defer state.transition.unlock()
	if !state.idleLongerThan(manager.now(), idle) {
		return nil
	}
	return manager.stopLocked(ctx, name)
}

// StopAll は、この engine が起こした経路をすべて、並べて畳む。engine を終える
// ときに使う。
//
// ひとつずつ畳むと、コンテナの停止を待つ時間が経路の数だけ積み重なる。
func (manager *Manager) StopAll(ctx context.Context) {
	var stopping sync.WaitGroup
	for _, name := range manager.names() {
		if !manager.state(name).isRunning() {
			continue
		}
		stopping.Add(1)
		go func() {
			defer stopping.Done()
			_ = manager.Stop(ctx, name)
		}()
	}
	stopping.Wait()
}

// names は、この engine が触ったプロファイルの名前を返す。
func (manager *Manager) names() []string {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	names := make([]string, 0, len(manager.routes))
	for name := range manager.routes {
		names = append(names, name)
	}
	return names
}

// Start は、プロファイル profileName のコンテナが経路を提供している状態にする。設定と
// シークレットは、起動と停止の鍵を取ってから source で読む。
//
// 呼び出し側の ctx が終わるか、Close が呼ばれるか、停止を頼まれたら、起動を打ち切って
// コンテナを片付ける。鍵を待っているあいだに停止を頼まれた起動は、何もせずに終える。
// 停止と Close で打ち切った起動は、停止の理由（ErrRouteDisconnected か ErrRouteStopped）を
// 返す。それ以外で ctx が終わって打ち切った起動は、ctx.Err()（context.Canceled など）を
// 含むエラーを返す。
func (manager *Manager) Start(ctx context.Context, profileName string, source RouteSource) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	state := manager.state(profileName)
	// 鍵を待つ前に控える。待っているあいだに停止を頼まれたら、この起動はやめる。
	stops := state.stopsRequested()
	ctx = manager.recording(ctx, profileName)
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	manager.describeDocker(ctx)
	ctx, cancel := manager.bound(ctx)
	defer cancel(nil)
	if err := manager.orphans.wait(ctx); err != nil {
		return stopCauseOr(ctx, err)
	}
	if err := state.transition.lock(ctx); err != nil {
		return stopCauseOr(ctx, err)
	}
	defer state.transition.unlock()
	if err := state.beginStart(stops, cancel); err != nil {
		connectionlog.Say(ctx, connectionlog.Detailed, "起動を待っているあいだにVPN経路が停止されたため、起動を中止しました。")
		return err
	}
	defer state.endStart()
	if err := manager.startLocked(ctx, profileName, source); err != nil {
		// ctx の取り消しで docker を止めた段では、docker の失敗（signal: killed）しか
		// 残らない。取り消したのか故障したのかを、呼び出し側が見分けられるようにする。
		if ended := ctx.Err(); ended != nil && !errors.Is(err, ended) {
			err = fmt.Errorf("%w: %w", ended, err)
		}
		return stopCauseOr(ctx, err)
	}
	return nil
}

// startLocked は、Start の本体である。起動と停止の鍵を握り、停止で打ち切れるように
// してから呼ぶ。
func (manager *Manager) startLocked(ctx context.Context, profileName string, source RouteSource) error {
	state := manager.state(profileName)
	// 起動できても、できずにコンテナを片付けても、経路の状態は docker から読んだ
	// ものより新しくなる。
	defer manager.noteChange(state)
	profile, secrets, err := source()
	if err != nil {
		return err
	}
	if profile.Name != profileName {
		return fmt.Errorf("%w: %s", ErrProfileName, profile.Name)
	}
	if err := profile.Validate(); err != nil {
		return err
	}
	// 起動を求められたので、利用者が切断した経路でも、もう切断したままにはしない。
	state.setDisconnected(false)

	name := manager.containerName(profileName)
	ours, err := manager.requireOurContainer(ctx, name, profileName)
	if err != nil {
		return err
	}
	route := newRouteIdentity(profile, secrets)
	if ours && state.serves(route) {
		running, err := manager.containerRunning(ctx, name)
		if err != nil {
			return err
		}
		if running {
			// 起動を求められた経路は、いまから使われる。CLI は起動を求めてから
			// engine の中継へ繋ぐので、そのあいだに停止されないよう、無操作の
			// 起点をいまにする。
			state.touch(manager.now())
			connectionlog.Say(ctx, connectionlog.Detailed, "起動済みのVPN経路（コンテナ「%s」）を使います。", name)
			return nil
		}
	}
	// 設定かシークレットが変わった、または止まっている。作り直す方が、半端な状態を
	// 残すより分かりやすい。
	manager.closeRelay(state)
	if ours {
		connectionlog.Say(ctx, connectionlog.Detailed, "設定やシークレットが変わったか、停止していたため、コンテナ「%s」を作り直します。", name)
		manager.stopContainer(ctx, name)
	}
	defer state.enterPhase("")
	connectionlog.Say(ctx, connectionlog.Brief, "VPN経路「%s」を起動します（%s）。", profile.Name, profile.Backend)
	started := time.Now()
	if err := manager.start(ctx, profile, secrets, state.enterPhase); err != nil {
		elapsed := connectionlog.Elapsed(time.Since(started))
		if stopCauseOf(ctx) != nil {
			// 停止で打ち切った起動は失敗ではない。失敗の詳細は経路の記録に残さない。
			connectionlog.Say(ctx, connectionlog.Brief, "VPN経路の起動を中止しました（%s）。", elapsed)
			return err
		}
		connectionlog.Say(ctx, connectionlog.Detailed, "VPN経路の起動に失敗しました（%s）。", elapsed)
		// 元のエラーは、接続ログでは接続の側（sshclient の「失敗の詳細」）が書く。
		// こちらからも書くと同じ文が2回並ぶので、経路の記録にだけ残す。
		state.record.Write(connectionlog.Detailed, fmt.Sprintf("失敗の詳細：%v", err))
		return err
	}
	tunnel := manager.tunnelStatus(profileName)
	connectionlog.Say(ctx, connectionlog.Brief, "VPNに接続しました（%s、インターフェース：%s、アドレス：%s、%s）。",
		profile.Backend, tunnel.Interface, tunnel.Address, connectionlog.Elapsed(time.Since(started)))
	relay, err := openEngineRelay(
		filepath.Join(manager.routeDirectory(profileName), engineRelaySocketName),
		func(ctx context.Context, address string) (net.Conn, error) {
			return manager.dialRelay(ctx, profile, address)
		},
		relayRequestTimeout,
	)
	if err != nil {
		manager.stopContainer(ctx, name)
		return err
	}
	state.markStarted(route, relay, manager.now())
	return nil
}

// bound は、呼び出し側の ctx と、この Manager の寿命の両方に従う ctx を返す。寿命が
// 終わるのは sshcエンジンの終了なので、ErrRouteStopped で取り消す。返す関数は、
// 理由を添えて ctx を取り消す（nil なら context.Canceled）。
func (manager *Manager) bound(ctx context.Context) (context.Context, context.CancelCauseFunc) {
	bound, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(manager.lifetime, func() { cancel(ErrRouteStopped) })
	return bound, func(cause error) {
		stop()
		cancel(cause)
	}
}

// closeRelay は、engine が差し出している中継をやめる。
func (manager *Manager) closeRelay(state *routeState) {
	if relay := state.markStopped(); relay != nil {
		_ = relay.close()
	}
}

// Stop は、このプロファイルのコンテナを止め、中継のソケットを消す。
func (manager *Manager) Stop(ctx context.Context, profileName string) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	state := manager.state(profileName)
	state.requestStop(ErrRouteStopped)
	if err := state.transition.lock(ctx); err != nil {
		return err
	}
	defer state.transition.unlock()
	return manager.stopLocked(ctx, profileName)
}

// Forget は、プロファイルを削除・改名したあとで、その名前の経路を止め、記録と
// 用意できなかったときのログも捨てる。
//
// 同じ名前でプロファイルを作り直したときに、前のプロファイルのサーバー名やエラーを
// 新しいもののように見せない。名前の状態そのものは消さない。消す前に始まっていた
// 起動が、見えなくなった状態に経路を付けて残すからである。
func (manager *Manager) Forget(ctx context.Context, profileName string) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	state := manager.state(profileName)
	state.requestStop(ErrRouteStopped)
	if err := state.transition.lock(ctx); err != nil {
		return err
	}
	defer state.transition.unlock()
	defer state.forgetHistory()
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	return manager.stopLocked(ctx, profileName)
}

// Disconnect は、利用者の求めで経路を切断する。
//
// Stop と違い、利用者が切断したことを覚えておく（Disconnected）。経路を止めると、
// それを通っていた接続も切れる。その自動再接続で、切断した経路を起動し直さない
// ためである。停止より先に覚える。停止の途中に切れた接続の再接続が、先に経路を
// 起動し直さないようにする。
func (manager *Manager) Disconnect(ctx context.Context, profileName string) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	state := manager.state(profileName)
	state.requestStop(ErrRouteDisconnected)
	if err := state.transition.lock(ctx); err != nil {
		return err
	}
	defer state.transition.unlock()
	state.setDisconnected(true)
	if err := manager.stopLocked(ctx, profileName); err != nil {
		// 止められなかった経路は動いているかもしれない。切断したとは覚えない。
		state.setDisconnected(false)
		return err
	}
	return nil
}

// Disconnected は、利用者がこの経路を切断し、まだ起動し直していないかを返す。
func (manager *Manager) Disconnected(profileName string) bool {
	return manager.state(profileName).isDisconnected()
}

// stopLocked は、Stop の本体である。起動と停止の鍵を握って呼ぶこと。
//
// 鍵を取ったあとは、呼び出し側が諦めても止めきる。取り消しで何も止めずに終わると、
// 経路が残る。docker が応えないときに止まったままにならないよう、上限は付ける。
func (manager *Manager) stopLocked(ctx context.Context, profileName string) error {
	defer manager.noteChange(manager.state(profileName))
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), inspectTimeout)
	defer cancel()
	name := manager.containerName(profileName)
	if _, err := manager.requireOurContainer(ctx, name, profileName); err != nil {
		return err
	}
	manager.closeRelay(manager.state(profileName))
	manager.stopContainer(ctx, name)
	return removeRouteFiles(manager.routeDirectory(profileName))
}

// command は、使える docker をひとつだけ探して覚える。
func (manager *Manager) command(ctx context.Context) (dockerCommand, error) {
	if err := manager.lookup.lock(ctx); err != nil {
		return dockerCommand{}, err
	}
	defer manager.lookup.unlock()
	if manager.found {
		return manager.docker, nil
	}
	if !manager.loaded {
		manager.variables, manager.pathSource = manager.loadEnvironment(ctx)
		manager.loaded = true
	}
	command, err := findDocker(ctx, manager.variables)
	if err != nil {
		manager.lookupFailure, manager.lookupFailedAt = err, manager.now()
		connectionlog.Say(ctx, connectionlog.Detailed, "dockerを探したPATHの取得元：%s", manager.pathSource)
		connectionlog.Say(ctx, connectionlog.Full, "PATH：%s", manager.searchedPath())
		connectionlog.Say(ctx, connectionlog.Detailed, "Dockerを使用できません：%v", err)
		return dockerCommand{}, err
	}
	manager.docker, manager.found, manager.lookupFailure = command, true, nil
	return command, nil
}

// commandForReading は、経路の状態を読むための docker を返す。直前に探して使え
// なかったなら、routeReadingLifetime のあいだは探し直さずに同じ理由を返す。
//
// Docker Desktop を起動していない mac では、docker info は失敗するまでに1秒以上
// かかる。画面の読み直しのたびに探し直すと、そのたびに一覧が遅れる。経路の起動
// のように利用者が求めた操作は、command でその場で探し直す。
func (manager *Manager) commandForReading(ctx context.Context) (dockerCommand, error) {
	if err := manager.lookup.lock(ctx); err != nil {
		return dockerCommand{}, err
	}
	failure, failedAt := manager.lookupFailure, manager.lookupFailedAt
	manager.lookup.unlock()
	if failure != nil && manager.now().Sub(failedAt) < routeReadingLifetime {
		return dockerCommand{}, failure
	}
	return manager.command(ctx)
}

// loadEnvironment は、docker を探して起動する環境と、その取得元の説明を返す。
// 読めなければ nil を返し、engine の環境で探す。
func (manager *Manager) loadEnvironment(ctx context.Context) ([]string, string) {
	if manager.environment == nil {
		return nil, "sshcエンジンの環境"
	}
	variables, err := manager.environment(ctx)
	if err != nil {
		return nil, fmt.Sprintf("sshcエンジンの環境（ログインシェルから取得できませんでした：%v）", err)
	}
	return variables, "ログインシェル"
}

// searchedPath は、docker を探した PATH である。lookup を握って呼ぶこと。
func (manager *Manager) searchedPath() string {
	if manager.variables == nil {
		return os.Getenv("PATH")
	}
	path, _ := platform.LookupEnvironment(manager.variables, "PATH")
	return path
}

// describeDocker は、使う docker を接続ログの debug2 に書く。経路の起動と接続の
// たびに書く。docker は一度だけ探すので、探したときの ctx には接続ログが無いことがある。
func (manager *Manager) describeDocker(ctx context.Context) {
	if !connectionlog.Enabled(ctx, connectionlog.Detailed) {
		return
	}
	if manager.lookup.lock(ctx) != nil {
		return
	}
	path, summary, source, searched := manager.docker.path, manager.docker.summary, manager.pathSource, manager.searchedPath()
	manager.lookup.unlock()
	connectionlog.Say(ctx, connectionlog.Detailed, "docker：%s（%s）", path, summary)
	connectionlog.Say(ctx, connectionlog.Detailed, "dockerを探したPATHの取得元：%s", source)
	connectionlog.Say(ctx, connectionlog.Full, "PATH：%s", searched)
}

func (manager *Manager) state(profileName string) *routeState {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	state, present := manager.routes[profileName]
	if !present {
		state = &routeState{}
		manager.routes[profileName] = state
	}
	return state
}
