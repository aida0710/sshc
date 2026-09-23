package vpn

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
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

	mutex    sync.Mutex
	docker   dockerCommand
	found    bool
	sessions map[string]*sessionState
}

// sessionState は、プロファイルひとつぶんの進行中の状態である。
type sessionState struct {
	mutex sync.Mutex
	// started は、いまコンテナが提供している設定である。設定が変わったら
	// 作り直す。古い設定のまま繋ぎ続けると、利用者が直した先へ行かない。
	started Profile
	running bool
}

// Status は、プロファイルひとつの状態である。
type Status struct {
	Name string
	// Running は、コンテナが動いていることを表す。
	Running bool
	// RelaySocket は、中継のソケットの場所である。開いていなければ空になる。
	// 値があれば、トンネルと経路とパケットフィルタは用意できている。
	RelaySocket string
	// Target は、このセッションが繋ぐ先である。
	Target string
}

// New は、VPNセッションの管理を作る。
//
// directory は、engineだけが読み書きするディレクトリの下を渡す。
func New(directory string, owner int) *Manager {
	return &Manager{directory: directory, owner: owner, sessions: map[string]*sessionState{}}
}

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
	if err := manager.Start(ctx, profile, secrets); err != nil {
		return nil, err
	}
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", manager.socketPath(profile.Name))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	return connection, nil
}

// Start は、このプロファイルのコンテナが経路を提供している状態にする。
func (manager *Manager) Start(ctx context.Context, profile Profile, secrets Secrets) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	state := manager.state(profile.Name)
	state.mutex.Lock()
	defer state.mutex.Unlock()

	name := containerName(profile.Name, manager.owner)
	ours, err := manager.requireOurContainer(ctx, name, profile.Name)
	if err != nil {
		return err
	}
	if ours && state.running && state.started == profile && manager.relayPresent(profile.Name) {
		running, err := manager.containerRunning(ctx, name)
		if err == nil && running {
			return nil
		}
	}
	if ours {
		// 設定が変わった、または止まっている。作り直す方が、半端な状態を
		// 残すより分かりやすい。
		_ = manager.stopContainer(ctx, name)
	}
	state.running = false
	if err := manager.start(ctx, profile, secrets); err != nil {
		return err
	}
	state.started = profile
	state.running = true
	return nil
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
	state.mutex.Lock()
	defer state.mutex.Unlock()

	name := containerName(profileName, manager.owner)
	if _, err := manager.requireOurContainer(ctx, name, profileName); err != nil {
		return err
	}
	if err := manager.stopContainer(ctx, name); err != nil {
		return err
	}
	state.running = false
	return removeIfPresent(manager.socketPath(profileName))
}

// Status は、このプロファイルの状態を返す。
func (manager *Manager) Status(ctx context.Context, profileName string) (Status, error) {
	if err := validateProfileName(profileName); err != nil {
		return Status{}, err
	}
	if _, err := manager.command(ctx); err != nil {
		return Status{}, err
	}
	name := containerName(profileName, manager.owner)
	status := Status{Name: profileName}
	if manager.relayPresent(profileName) {
		status.RelaySocket = manager.socketPath(profileName)
	}
	ours, err := manager.requireOurContainer(ctx, name, profileName)
	if err != nil {
		return Status{}, err
	}
	if !ours {
		return status, nil
	}
	running, err := manager.containerRunning(ctx, name)
	if err != nil {
		return Status{}, err
	}
	status.Running = running
	format := "{{index .Config.Labels \"" + targetLabel + "\"}}"
	if output, err := manager.docker.output(ctx, "container", "inspect", "--format", format, name); err == nil {
		status.Target = strings.TrimSpace(output)
	}
	return status, nil
}

// DiscardOrphans は、前回のengineが残したコンテナを止める。
//
// 引き継がない。そのコンテナがどの設定で経路を張ったのかを確かめられない以上、
// 使い続けるより止める方が安全である。
func (manager *Manager) DiscardOrphans(ctx context.Context) error {
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	output, err := manager.docker.output(ctx, "ps", "--all", "--quiet",
		"--filter", "label="+ownerLabel+"="+strconv.Itoa(manager.owner))
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(output) {
		_ = manager.stopContainer(ctx, id)
	}
	return nil
}

// relayPresent は、中継のソケットがあるかを見る。
func (manager *Manager) relayPresent(profileName string) bool {
	_, err := os.Stat(manager.socketPath(profileName))
	return err == nil
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
