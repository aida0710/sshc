package vpn

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// engine が経路ごとに差し出す中継である。
//
// CLI（`sshc <接続先>`）とホストの ssh（`sshc vpn proxy`）は、コンテナの中継では
// なくここへ繋ぐ。engine を通すのは、どの入口からの接続も同じように数えるため
// である。数えられない接続があると、使っている経路を無操作と見なして畳んでしまう。

// engineRelaySocketName は、engine が差し出す中継の名前である。
const engineRelaySocketName = "engine.sock"

// maxSocketPathLength は、Unix ソケットのパスに書ける長さである。Linux の
// sun_path は 108 バイトで、終わりの NUL に 1 バイト使う。
const maxSocketPathLength = 107

// ErrSocketPath は、中継のソケットを置く場所のパスが長すぎることを表す。
var ErrSocketPath = errors.New("the vpn relay socket path is too long")

// engineRelay は、ひとつの経路の中継の待ち受けである。
type engineRelay struct {
	path     string
	listener net.Listener
	// accepting は、受け付けの goroutine が終わると閉じる。
	accepting chan struct{}
}

// openEngineRelay は、path で待ち受け、受けた接続を dial が返す接続とつなぐ。
//
// dial は、数えられる接続を返す。受けた接続が終われば、その接続も閉じる。
func openEngineRelay(path string, dial func() (net.Conn, error)) (*engineRelay, error) {
	if err := removeIfPresent(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	// ディレクトリは 0700 だが、ソケットそのものも利用者だけにする。
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	relay := &engineRelay{path: path, listener: listener, accepting: make(chan struct{})}
	go relay.accept(dial)
	return relay, nil
}

func (relay *engineRelay) accept(dial func() (net.Conn, error)) {
	defer close(relay.accepting)
	for {
		client, err := relay.listener.Accept()
		if err != nil {
			return
		}
		go pipeRelay(client, dial)
	}
}

// pipeRelay は、受けた接続と経路の接続のあいだで、両方向へバイト列を運ぶ。
func pipeRelay(client net.Conn, dial func() (net.Conn, error)) {
	defer func() { _ = client.Close() }()
	route, err := dial()
	if err != nil {
		return
	}
	defer func() { _ = route.Close() }()
	var copying sync.WaitGroup
	copying.Add(2)
	go func() {
		defer copying.Done()
		_, _ = io.Copy(route, client)
		closeWrite(route)
	}()
	go func() {
		defer copying.Done()
		_, _ = io.Copy(client, route)
		closeWrite(client)
	}()
	copying.Wait()
}

// closeWrite は、送る側が終わったことを相手へ伝える。伝えないと、相手は入力の
// 終わりを待ち続ける。
func closeWrite(connection net.Conn) {
	if half, ok := connection.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
		return
	}
	if counted, ok := connection.(*countedConnection); ok {
		closeWrite(counted.Conn)
	}
}

// close は、待ち受けをやめ、ソケットを消す。通っている接続は、経路の側が
// 閉じれば終わる。
func (relay *engineRelay) close() error {
	err := relay.listener.Close()
	<-relay.accepting
	if removeErr := removeIfPresent(relay.path); removeErr != nil {
		return removeErr
	}
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// requireSocketPaths は、この経路のソケットのパスが長さの上限に収まるかを確かめる。
//
// 収まらないと、コンテナの中継か engine の中継のどちらかが、分かりにくい失敗の
// 仕方で開けない。workspace が深い場所にあると起こりうる。
func requireSocketPaths(directory string) error {
	for _, name := range []string{relaySocketName, engineRelaySocketName} {
		if path := filepath.Join(directory, name); len(path) > maxSocketPathLength {
			return fmt.Errorf("%w: %d > %d", ErrSocketPath, len(path), maxSocketPathLength)
		}
	}
	return nil
}
