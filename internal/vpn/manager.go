package vpn

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"time"
)

// Manager は、この engine が持つVPNセッションの全体である。
//
// Dockerが無い機械でも作れる。使えるかどうかは、使う時点で確かめて理由を返す。
// engineの起動が、入っていないかもしれないものに依存しないためである。
type Manager struct {
	// directory は、プロファイルごとの経路の置き場所（routeDirectory）を置く場所である。
	directory string
	// owner は、このengineを動かしている利用者である。コンテナとイメージの名前と
	// 札に使う。
	owner int
	// workspace は、この engine の workspace を表す短い識別子である。同じ利用者の
	// 別の workspace の engine が立てたコンテナと取り違えないために使う。
	workspace string

	// environment は、docker を探して起動する環境を返す。nil なら engine の環境を使う。
	environment Environment

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
	// 待つあいだも、経路の状態は mutex で読める。
	lookup sync.Mutex
	docker dockerCommand
	found  bool
	// variables は、一度読めたログインシェルの環境である。画面は VPN の一覧を
	// 読み直し続けるので、Docker が無いあいだも毎回シェルを起動しない。
	variables []string

	mutex    sync.Mutex
	sessions map[string]*sessionState
}

// New は、VPNセッションの管理を作る。
//
// directory は、engineだけが読み書きするディレクトリの下を渡す。workspace の
// 識別子は、この directory から決まる。environment は、docker を探して起動する
// 環境を返す（nil なら engine の環境）。
func New(directory string, owner int, environment Environment) *Manager {
	lifetime, cancel := context.WithCancel(context.Background())
	return &Manager{
		directory: directory, owner: owner, workspace: workspaceIdentity(directory),
		environment: environment, now: time.Now,
		lifetime: lifetime, close: cancel, orphans: newOrphanGate(),
		sessions: map[string]*sessionState{},
	}
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

// Dial は、このプロファイルの経路を通して、address（`host:port`）へのTCP接続を返す。
//
// address は、このプロファイルを付けた接続の HostName と Port である。必要なら
// コンテナを起こし、経路ができるまで待つ。返るのはコンテナの中の中継の標準入出力で
// あり、その先のTCPはコンテナのトンネルを通る。
func (manager *Manager) Dial(ctx context.Context, profile Profile, secrets Secrets, address string) (net.Conn, error) {
	destination, err := profile.Destination(address)
	if err != nil {
		return nil, err
	}
	// 起動より先に借りる。起動が終わってから借りるまでのあいだに、無操作と
	// 見なされて停止されないためである。
	state := manager.state(profile.Name)
	state.borrow()
	if err := manager.Start(ctx, profile, secrets); err != nil {
		state.release(manager.now())
		return nil, err
	}
	return manager.connectCounted(ctx, state, profile.Name, destination)
}

// dialRelay は、engine の中継が受けた接続のために、起動済みの経路で接続先へ繋ぐ。
func (manager *Manager) dialRelay(ctx context.Context, profile Profile, address string) (net.Conn, error) {
	destination, err := profile.Destination(address)
	if err != nil {
		return nil, err
	}
	state := manager.state(profile.Name)
	state.borrow()
	return manager.connectCounted(ctx, state, profile.Name, destination)
}

// connectCounted は、借りを済ませた経路で接続先へ繋ぎ、閉じたときに借りを返す
// 接続にする。繋げなければ借りを返す。
func (manager *Manager) connectCounted(
	ctx context.Context, state *sessionState, profileName string, destination Endpoint,
) (net.Conn, error) {
	connection, err := manager.connectTarget(ctx, profileName, destination)
	if err != nil {
		state.release(manager.now())
		return nil, err
	}
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
	state.transition.Lock()
	defer state.transition.Unlock()
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
	names := make([]string, 0, len(manager.sessions))
	for name := range manager.sessions {
		names = append(names, name)
	}
	return names
}

// Start は、このプロファイルのコンテナが経路を提供している状態にする。
//
// 呼び出し側の ctx が終わるか、Close が呼ばれたら、起動を打ち切ってコンテナを
// 片付ける。
func (manager *Manager) Start(ctx context.Context, profile Profile, secrets Secrets) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	ctx, cancel := manager.bound(ctx)
	defer cancel()
	if err := manager.orphans.wait(ctx); err != nil {
		return err
	}
	state := manager.state(profile.Name)
	state.transition.Lock()
	defer state.transition.Unlock()

	name := manager.containerName(profile.Name)
	ours, err := manager.requireOurContainer(ctx, name, profile.Name)
	if err != nil {
		return err
	}
	if ours && state.serves(profile) {
		running, err := manager.containerRunning(ctx, name)
		if err != nil {
			return err
		}
		if running {
			// 起動を求められた経路は、いまから使われる。CLI は起動を求めてから
			// engine の中継へ繋ぐので、そのあいだに停止されないよう、無操作の
			// 起点をいまにする。
			state.touch(manager.now())
			return nil
		}
	}
	// 設定が変わった、または止まっている。作り直す方が、半端な状態を残すより
	// 分かりやすい。
	manager.closeRelay(state)
	if ours {
		manager.stopContainer(ctx, name)
	}
	defer state.enterPhase("")
	if err := manager.start(ctx, profile, secrets, state.enterPhase); err != nil {
		return err
	}
	relay, err := openEngineRelay(
		filepath.Join(manager.routeDirectory(profile.Name), engineRelaySocketName),
		func(ctx context.Context, address string) (net.Conn, error) {
			return manager.dialRelay(ctx, profile, address)
		},
	)
	if err != nil {
		manager.stopContainer(ctx, name)
		return err
	}
	state.markStarted(profile, relay, manager.now())
	return nil
}

// bound は、呼び出し側の ctx と、この Manager の寿命の両方に従う ctx を返す。
func (manager *Manager) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(manager.lifetime, cancel)
	return bound, func() {
		stop()
		cancel()
	}
}

// closeRelay は、engine が差し出している中継をやめる。
func (manager *Manager) closeRelay(state *sessionState) {
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
	state.transition.Lock()
	defer state.transition.Unlock()
	return manager.stopLocked(ctx, profileName)
}

// stopLocked は、Stop の本体である。起動と停止の鍵を握って呼ぶこと。
func (manager *Manager) stopLocked(ctx context.Context, profileName string) error {
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
	manager.lookup.Lock()
	defer manager.lookup.Unlock()
	if manager.found {
		return manager.docker, nil
	}
	if manager.variables == nil {
		manager.variables = manager.loadEnvironment(ctx)
	}
	command, err := findDocker(ctx, manager.variables)
	if err != nil {
		return dockerCommand{}, err
	}
	manager.docker, manager.found = command, true
	return command, nil
}

// loadEnvironment は、docker を探して起動する環境を読む。読めなければ nil を返し、
// engine の環境で探す。
func (manager *Manager) loadEnvironment(ctx context.Context) []string {
	if manager.environment == nil {
		return nil
	}
	variables, err := manager.environment(ctx)
	if err != nil {
		return nil
	}
	return variables
}

func (manager *Manager) state(profileName string) *sessionState {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	state, present := manager.sessions[profileName]
	if !present {
		state = &sessionState{}
		manager.sessions[profileName] = state
	}
	return state
}
