package remoteengine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sync"
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
	// LocalVersion は、このマシンの sshc のバージョンである。実行先のエンジンは同じ
	// バージョンでなければ使わない。
	LocalVersion string
	// PeerMayBelongToAnotherUser は、TCP の接続が、このマシンの別の OS ユーザーのもの
	// かもしれないかを返す。nil なら確かめない。
	PeerMayBelongToAnotherUser func(client, server netip.AddrPort) bool
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

	mutex  sync.Mutex
	hosts  map[string]*remoteHost
	closed bool
}

func NewManager(options ManagerOptions) *Manager {
	return &Manager{options: options, hosts: map[string]*remoteHost{}}
}

// host は alias の remoteHost を返す。無ければ作る。mutex を持って呼ぶ。
func (m *Manager) host(alias string) *remoteHost {
	target := m.hosts[alias]
	if target == nil {
		target = &remoteHost{link: newHostLink(func(ctx context.Context) (Link, error) {
			return m.options.Connect(ctx, alias)
		})}
		m.hosts[alias] = target
	}
	return target
}

// Engines は、実行先に足したエンジンの alias を、足した順に返す。
func (m *Manager) Engines() ([]string, error) {
	return m.options.Store.Engines()
}

// AddEngine は、alias で動いているエンジンを実行先に足す。足す前に、接続できること、
// 接続先のエンジンが動いていて同じバージョンであることを確かめる。
func (m *Manager) AddEngine(ctx context.Context, alias string) error {
	engine, err := m.hostEngine(alias)
	if err != nil {
		return err
	}
	if _, err := engine.current(ctx); err != nil {
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
		target.engine = newRemoteEngine(target.link, m.options.LocalVersion)
	}
	return target.engine, nil
}

// Open は、alias で動いているエンジンの入口の URL を 1 件発行させ、このマシンの中継を
// 通して開ける URL にして返す。中継がまだ無ければ待ち受けを始める。
//
// 開くたびに、接続先で入口の URL を発行させ直す。URL は一度しか使えず、接続先の
// エンジンが起動し直していればポートも変わっているからである。
func (m *Manager) Open(ctx context.Context, alias string) (string, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.closed {
		return "", ErrClosed
	}
	target := m.host(alias)
	if target.relay == nil {
		target.relay = newRelay(target.link)
	}
	opened, err := m.issueEntrance(ctx, target.link)
	if err != nil {
		m.dropIdleRelay(alias, target)
		return "", err
	}
	target.relay.setRemotePort(opened.port)
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

// issueEntrance は、接続先で入口の URL を 1 件発行させる。
func (m *Manager) issueEntrance(ctx context.Context, link *hostLink) (entrance, error) {
	live, err := link.live(ctx)
	if err != nil {
		return entrance{}, err
	}
	result, err := live.Run(ctx, EntranceCommand, EntranceLimit)
	switch {
	case err != nil:
		return entrance{}, err
	case result.ExitCode == notInstalledExit:
		return entrance{}, ErrNotInstalled
	case result.ExitCode != 0:
		return entrance{}, ErrNoEntrance
	}
	opened, err := parseEntrance(result.Stdout)
	if err != nil {
		return entrance{}, err
	}
	// URL を返す前に、接続先のエンジンへ届くかを一度確かめる。SSH サーバーが TCP を
	// 開かない設定だと、URL を開いたブラウザが理由の分からない 502 を受け取るだけになる。
	probe, err := live.DialContext(ctx, "tcp", loopbackHost(opened.port))
	if err != nil {
		return entrance{}, fmt.Errorf("%w: %v", ErrForwardingRefused, err)
	}
	_ = probe.Close()
	return opened, nil
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

// Close は、すべての中継と取り次ぎを閉じる。以後の要求は ErrClosed で断る。
func (m *Manager) Close() {
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
