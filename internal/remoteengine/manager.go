package remoteengine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"time"
)

var (
	// ErrClosed は、エンジンが止まりかけていて、接続先を開けないことを表す。
	ErrClosed = errors.New("the engine is stopping")
	// ErrNotAdded は、実行先に足していない接続先を指したことを表す。
	ErrNotAdded = errors.New("the remote engine has not been added")
)

// ConnectError は、接続先への SSH の接続ができなかったことを表す。Err が理由である。
type ConnectError struct{ Err error }

func (e *ConnectError) Error() string { return "connect to the remote host: " + e.Err.Error() }
func (e *ConnectError) Unwrap() error { return e.Err }

// ManagerOptions は、Manager が要るものである。
type ManagerOptions struct {
	// Connect は、alias へ非対話の SSH の接続を開く。
	Connect func(ctx context.Context, alias string) (Link, error)
	Store   *Store
	// LocalVersion は、このマシンの sshc のバージョンである。互換でない実行先を断るときに
	// 添える。使えるかどうかは、バージョンではなく Compatibility で決める。
	LocalVersion string
	// PeerMayBelongToAnotherUser は、TCP の接続が、このマシンの別の OS ユーザーのもの
	// かもしれないかを返す。nil なら確かめない。
	PeerMayBelongToAnotherUser func(client, server netip.AddrPort) bool
	// Logger は、接続先への SSH の接続の試みと結果を書く。nil なら書かない。
	Logger *slog.Logger
}

// remoteHost は、接続先ひとつについてこのエンジンが持つものである。SSH の接続はひとつで、
// 別タブの中継と、実行先としての取り次ぎが共有する。
type remoteHost struct {
	link   *hostLink
	relay  *relay
	engine *remoteEngine
}

// Manager は、接続先のエンジンへの中継と取り次ぎを持つ。寿命はこのマシンのエンジンと同じである。
type Manager struct {
	options ManagerOptions
	// answerLimit は forwardedAnswerLimit である。試験だけが短くする。
	answerLimit time.Duration
	// linkLimit は linkAttemptLimit、sessionLimit は sessionAnswerLimit である。試験だけが短くする。
	linkLimit    time.Duration
	sessionLimit time.Duration

	mutex  sync.Mutex
	hosts  map[string]*remoteHost
	closed bool
	// actions は、接続先ごとの、最後に始めた操作の記録である。SSH の接続を閉じても残し、
	// 終わった操作の出力も読めるようにする。
	actions map[string]*actionLog
	// actionContext は、走っている操作の寿命である。Close が stopActions で取り消す。
	actionContext  context.Context
	stopActions    context.CancelFunc
	actionsRunning sync.WaitGroup
}

func NewManager(options ManagerOptions) *Manager {
	actionContext, stopActions := context.WithCancel(context.Background())
	return &Manager{
		options: options, answerLimit: forwardedAnswerLimit, linkLimit: linkAttemptLimit, sessionLimit: sessionAnswerLimit,
		hosts: map[string]*remoteHost{}, actions: map[string]*actionLog{},
		actionContext: actionContext, stopActions: stopActions,
	}
}

// host は alias の remoteHost を返す。無ければ作る。mutex を持って呼ぶ。
func (m *Manager) host(alias string) *remoteHost {
	target := m.hosts[alias]
	if target == nil {
		target = &remoteHost{link: newHostLink(func(ctx context.Context) (Link, error) {
			return m.connect(ctx, alias)
		}, m.linkLimit)}
		m.hosts[alias] = target
	}
	return target
}

// connect は、alias へ SSH の接続を開き、試みと結果をログに書く。接続先が応答しないとき、
// 画面に出る一行だけでは、いつ何回試してどれだけ待ったかが分からないからである。
func (m *Manager) connect(ctx context.Context, alias string) (Link, error) {
	started := time.Now()
	link, err := m.options.Connect(ctx, alias)
	if m.options.Logger == nil {
		return link, err
	}
	elapsed := time.Since(started).Round(time.Millisecond)
	if err != nil {
		m.options.Logger.Warn("could not connect to the remote host", "alias", alias, "elapsed", elapsed, "error", err)
	} else {
		m.options.Logger.Info("connected to the remote host", "alias", alias, "elapsed", elapsed)
	}
	return link, err
}

// Engines は、実行先に足したエンジンの alias を、足した順に返す。
func (m *Manager) Engines() ([]string, error) {
	return m.options.Store.Engines()
}

// Connected は、alias の実行先へ、SSH の接続と接続先のエンジンの資格の両方を今持っているかを
// 答える。持っていれば、取り次ぎは繋ぎ直しも認証もせずに届く。繋ぎには行かない。
func (m *Manager) Connected(alias string) bool {
	m.mutex.Lock()
	target := m.hosts[alias]
	if target == nil || target.engine == nil {
		m.mutex.Unlock()
		return false
	}
	link, engine := target.link, target.engine
	m.mutex.Unlock()
	return link.connected() && engine.attached()
}

// AddEngine は、alias で動いているエンジンを実行先に足す。足す前に、接続できること、
// 接続先のエンジンが動いていることを確かめる。互換でないエンジンも足す。
// 画面から更新できるのは、実行先に足したエンジンだけだからである。
func (m *Manager) AddEngine(ctx context.Context, alias string) error {
	engine, err := m.hostEngine(alias)
	if err != nil {
		return err
	}
	var incompatible *IncompatibleEngineError
	if _, err := engine.current(ctx); err != nil && !errors.As(err, &incompatible) {
		return err
	}
	return m.options.Store.AddEngine(alias)
}

// RemoveEngine は、実行先からエンジンを外す。接続先のエンジンとそのターミナルは止めない。
// 別タブの中継が使っていなければ、SSH の接続も閉じる。
func (m *Manager) RemoveEngine(alias string) error {
	if err := m.options.Store.RemoveEngine(alias); err != nil {
		return err
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	target := m.hosts[alias]
	if target == nil || target.engine == nil {
		return nil
	}
	target.engine.close()
	target.engine = nil
	if target.relay == nil {
		target.link.close()
		delete(m.hosts, alias)
	}
	return nil
}

// EngineStatus は、実行先に足したエンジンの状態を尋ねる。
func (m *Manager) EngineStatus(ctx context.Context, alias string) (Status, error) {
	engine, err := m.engine(alias)
	if err != nil {
		return Status{}, err
	}
	return engine.status(ctx)
}

// engine は、実行先に足した alias の remoteEngine を返す。
func (m *Manager) engine(alias string) (*remoteEngine, error) {
	engines, err := m.options.Store.Engines()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(engines, alias) {
		return nil, ErrNotAdded
	}
	return m.hostEngine(alias)
}

// hostEngine は、alias の remoteEngine を返す。無ければ作る。このマシンのエンジンが
// 起動し直したあとは、保存した一覧の alias から作り直す。
func (m *Manager) hostEngine(alias string) (*remoteEngine, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	target := m.host(alias)
	if target.engine == nil {
		target.engine = newRemoteEngine(target.link, m.options.LocalVersion, m.sessionLimit)
	}
	return target.engine, nil
}

// Open は、alias で動いているエンジンの入口の URL を 1 件発行させ、このマシンの中継を
// 通して開ける URL にして返す。中継がまだ無ければ待ち受けを始める。
//
// 開くたびに、接続先で入口の URL を発行させ直す。URL は一度しか使えず、接続先の
// エンジンが起動し直していればポートも変わっているからである。
//
// 入口の発行は SSH で接続先へ行くので、mutex の外で行う。持ったままだと、接続先が
// 応答しないあいだ、ほかの接続先の要求もエンジンの停止も待たされる。
func (m *Manager) Open(ctx context.Context, alias string) (string, error) {
	m.mutex.Lock()
	if m.closed {
		m.mutex.Unlock()
		return "", ErrClosed
	}
	target := m.host(alias)
	m.mutex.Unlock()

	opened, remote, err := m.issueEntrance(ctx, target.link)

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed || m.hosts[alias] != target {
		// 発行を待つあいだに、エンジンが止まりかけたか、接続先が外された。
		return "", ErrClosed
	}
	if target.relay == nil {
		target.relay = newRelay(target.link)
	}
	if err != nil {
		m.dropIdleRelay(alias, target)
		return "", err
	}
	target.relay.setRemote(remote)
	if !target.relay.listening() {
		if err := m.listen(alias, target.relay); err != nil {
			m.dropIdleRelay(alias, target)
			return "", err
		}
	}
	return opened.at(target.relay.localPort), nil
}

// dropIdleRelay は、まだ待ち受けていない中継を捨てる。実行先としても使っていなければ、
// SSH の接続も閉じる。mutex を持って呼ぶ。
func (m *Manager) dropIdleRelay(alias string, target *remoteHost) {
	if target.relay.listening() {
		return
	}
	target.relay.close()
	target.relay = nil
	if target.engine == nil {
		target.link.close()
		delete(m.hosts, alias)
	}
}

// issueEntrance は、接続先で入口の URL を 1 件発行させ、その URL のエンジンの身元を
// handoff から読む。
func (m *Manager) issueEntrance(ctx context.Context, link *hostLink) (entrance, engineIdentity, error) {
	live, err := link.live(ctx)
	if err != nil {
		return entrance{}, engineIdentity{}, err
	}
	result, err := live.Run(ctx, RunRequest{Command: EntranceCommand, Limit: EntranceLimit})
	switch {
	case err != nil:
		return entrance{}, engineIdentity{}, err
	case result.ExitCode == notInstalledExit:
		return entrance{}, engineIdentity{}, ErrNotInstalled
	case result.ExitCode != 0:
		return entrance{}, engineIdentity{}, ErrNoEntrance
	}
	opened, err := parseEntrance(result.Stdout)
	if err != nil {
		return entrance{}, engineIdentity{}, err
	}
	// sshc open はエンジンを起こしてから URL を返すので、handoff は同じポートを指す。
	// 違えば、URL を発行したエンジンの身元を確かめられない。
	remote, err := readIdentity(ctx, link)
	if err != nil {
		return entrance{}, engineIdentity{}, err
	}
	if remote.port != opened.port {
		return entrance{}, engineIdentity{}, fmt.Errorf("%w: the UI URL is on port %d but the handoff names port %d", ErrUnproven, opened.port, remote.port)
	}
	// URL を返す前に、接続先のエンジンへ届き、身元を示せるかを一度確かめる。SSH サーバーが
	// TCP を開かない設定だと、URL を開いたブラウザが理由の分からない 502 を受け取るだけになる。
	probe, err := link.dialProven(ctx, remote)
	switch {
	case errors.Is(err, ErrUnproven):
		return entrance{}, engineIdentity{}, err
	case err != nil:
		return entrance{}, engineIdentity{}, fmt.Errorf("%w: %v", ErrForwardingRefused, err)
	}
	_ = probe.Close()
	return opened, remote, nil
}

// listen は、alias の中継を、前に使ったポートで待ち受ける。そのポートが塞がっていれば
// 別のポートを選んで覚え直す。
func (m *Manager) listen(alias string, target *relay) error {
	saved, found, err := m.options.Store.Port(alias)
	if err != nil {
		return err
	}
	listener, port, err := listenLoopback(saved)
	if err != nil && found {
		listener, port, err = listenLoopback(0)
	}
	if err != nil {
		return err
	}
	if err := m.options.Store.Remember(alias, port); err != nil {
		_ = listener.Close()
		return err
	}
	target.serve(sameUserListener{
		Listener: listener, mayBelongToAnotherUser: m.options.PeerMayBelongToAnotherUser,
	}, port)
	return nil
}

// Close は、すべての中継と取り次ぎを閉じ、走っている操作を止めて終わるまで待つ。
// 以後の要求は ErrClosed で断る。
func (m *Manager) Close() {
	// 操作を先に取り消す。SSH の接続を先に閉じると、操作は止められた理由ではなく
	// 接続の失敗で終わったと記録される。
	m.stopActions()
	m.closeHosts()
	m.actionsRunning.Wait()
}

func (m *Manager) closeHosts() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.closed = true
	for alias, target := range m.hosts {
		if target.relay != nil {
			target.relay.close()
		}
		if target.engine != nil {
			target.engine.close()
		}
		target.link.close()
		delete(m.hosts, alias)
	}
}
