package vpn

import (
	"context"
	"fmt"
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
	// directory は、プロファイルごとの中継ソケットを置く場所である。
	directory string
	// owner は、このengineを動かしている利用者である。コンテナの名前と札、
	// ソケットの持ち主に使う。
	owner int
	// workspace は、この engine の workspace を表す短い識別子である。同じ利用者の
	// 別の workspace の engine が立てたコンテナと取り違えないために使う。
	workspace string

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

	mutex    sync.Mutex
	docker   dockerCommand
	found    bool
	sessions map[string]*sessionState
}

// New は、VPNセッションの管理を作る。
//
// directory は、engineだけが読み書きするディレクトリの下を渡す。workspace の
// 識別子は、この directory から決まる。
func New(directory string, owner int) *Manager {
	lifetime, cancel := context.WithCancel(context.Background())
	return &Manager{
		directory: directory, owner: owner, workspace: workspaceIdentity(directory), now: time.Now,
		lifetime: lifetime, close: cancel, orphans: newOrphanGate(),
		sessions: map[string]*sessionState{},
	}
}

// Close は、進行中の起動を打ち切る。経路そのものは StopAll で畳む。
func (manager *Manager) Close() { manager.close() }

// Available は、この機械でVPN経路を使えるかを返す。
//
// ここで見るのは Docker だけである。トンネルのデバイスは backend ごとに違うので、
// そのプロファイルを起こすときに確かめる。使えない理由は隠さない。利用者が何を
// 用意すればよいかを決める情報である。
func (manager *Manager) Available(ctx context.Context) error {
	_, err := manager.command(ctx)
	return err
}

// Dial は、このプロファイルの接続先へのTCP接続を返す。
//
// 必要ならコンテナを起こし、経路ができるまで待つ。返るのはコンテナの中継への
// Unixソケットであり、その先のTCPはコンテナのトンネルを通る。
func (manager *Manager) Dial(ctx context.Context, profile Profile, secrets Secrets) (net.Conn, error) {
	if err := validateProfileName(profile.Name); err != nil {
		return nil, err
	}
	// 起動より先に借りる。起動が終わってから借りるまでのあいだに、無操作と
	// 見なされて畳まれないためである。
	state := manager.state(profile.Name)
	state.borrow()
	if err := manager.Start(ctx, profile, secrets); err != nil {
		state.release(manager.now())
		return nil, err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", manager.socketPath(profile.Name))
	if err != nil {
		state.release(manager.now())
		return nil, fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	return manager.counted(state, connection), nil
}

// counted は、閉じたときに借りを返す接続にする。借りは呼び出し側が先に済ませてある。
func (manager *Manager) counted(state *sessionState, connection net.Conn) net.Conn {
	return &countedConnection{Conn: connection, release: func() { state.release(manager.now()) }}
}

// dialRelay は、engine の中継が受けた接続のために、コンテナの中継へ繋ぐ。
func (manager *Manager) dialRelay(profileName string) (net.Conn, error) {
	state := manager.state(profileName)
	state.borrow()
	connection, err := net.Dial("unix", manager.socketPath(profileName))
	if err != nil {
		state.release(manager.now())
		return nil, err
	}
	return manager.counted(state, connection), nil
}

// StopIdle は、接続が一本も通っていない状態が idle を超えた経路を畳む。
//
// 畳まないままにすると、一度使った経路のコンテナが engine の寿命のあいだ
// 残り続ける。トンネルは使っているあいだだけあればよい。
func (manager *Manager) StopIdle(ctx context.Context, idle time.Duration) {
	now := manager.now()
	for _, name := range manager.names() {
		if manager.state(name).idleLongerThan(now, idle) {
			_ = manager.Stop(ctx, name)
		}
	}
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
		filepath.Join(manager.socketDirectory(profile.Name), engineRelaySocketName),
		func() (net.Conn, error) { return manager.dialRelay(profile.Name) },
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

	name := manager.containerName(profileName)
	if _, err := manager.requireOurContainer(ctx, name, profileName); err != nil {
		return err
	}
	manager.closeRelay(state)
	manager.stopContainer(ctx, name)
	return removeRouteFiles(manager.socketDirectory(profileName))
}

// command は、使える docker をひとつだけ探して覚える。
func (manager *Manager) command(ctx context.Context) (dockerCommand, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.found {
		return manager.docker, nil
	}
	command, err := findDocker(ctx)
	if err != nil {
		return dockerCommand{}, err
	}
	manager.docker, manager.found = command, true
	return command, nil
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
