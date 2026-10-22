package remoteengine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
)

// ErrClosed は、エンジンが止まりかけていて、中継を開けないことを表す。
var ErrClosed = errors.New("the engine is stopping")

// ConnectError は、接続先への SSH の接続ができなかったことを表す。Err が理由である。
type ConnectError struct{ Err error }

func (e *ConnectError) Error() string { return "connect to the remote host: " + e.Err.Error() }
func (e *ConnectError) Unwrap() error { return e.Err }

// ManagerOptions は、Manager が要るものである。
type ManagerOptions struct {
	// Connect は、alias へ非対話の SSH の接続を開く。
	Connect func(ctx context.Context, alias string) (Link, error)
	Ports   *PortStore
	// PeerMayBelongToAnotherUser は、TCP の接続が、このマシンの別の OS ユーザーのもの
	// かもしれないかを返す。nil なら確かめない。
	PeerMayBelongToAnotherUser func(client, server netip.AddrPort) bool
}

// Manager は、接続先ごとの中継を持つ。中継の寿命はエンジンと同じである。
type Manager struct {
	options ManagerOptions

	mutex  sync.Mutex
	relays map[string]*relay
	closed bool
}

func NewManager(options ManagerOptions) *Manager {
	return &Manager{options: options, relays: map[string]*relay{}}
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
	target := m.relays[alias]
	if target == nil {
		target = newRelay(func(ctx context.Context) (Link, error) {
			return m.options.Connect(ctx, alias)
		})
	}
	opened, err := m.issueEntrance(ctx, target)
	if err != nil {
		if !target.listening() {
			target.close()
		}
		return "", err
	}
	target.setRemotePort(opened.port)
	if !target.listening() {
		if err := m.listen(alias, target); err != nil {
			target.close()
			return "", err
		}
		m.relays[alias] = target
	}
	return opened.at(target.localPort), nil
}

// issueEntrance は、接続先で入口の URL を 1 件発行させる。
func (m *Manager) issueEntrance(ctx context.Context, target *relay) (entrance, error) {
	link, err := target.liveLink(ctx)
	if err != nil {
		return entrance{}, err
	}
	result, err := link.Run(ctx, EntranceCommand, EntranceLimit)
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
	probe, err := link.DialContext(ctx, "tcp", loopbackHost(opened.port))
	if err != nil {
		return entrance{}, fmt.Errorf("%w: %v", ErrForwardingRefused, err)
	}
	_ = probe.Close()
	return opened, nil
}

// listen は、alias の中継を、前に使ったポートで待ち受ける。そのポートが塞がっていれば
// 別のポートを選んで覚え直す。
func (m *Manager) listen(alias string, target *relay) error {
	saved, found, err := m.options.Ports.Port(alias)
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
	if err := m.options.Ports.Remember(alias, port); err != nil {
		_ = listener.Close()
		return err
	}
	target.serve(sameUserListener{
		Listener: listener, mayBelongToAnotherUser: m.options.PeerMayBelongToAnotherUser,
	}, port)
	return nil
}

// Close は、すべての中継を閉じる。以後の Open は ErrClosed で断る。
func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.closed = true
	for alias, target := range m.relays {
		target.close()
		delete(m.relays, alias)
	}
}
