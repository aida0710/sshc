package remoteengine

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// relayReadHeaderTimeout は、中継がブラウザの要求のヘッダーを読み終えるまでの上限である。
// 待ち受けは 127.0.0.1 だけだが、ヘッダーを送らずに居座る接続を残さない。
const relayReadHeaderTimeout = 10 * time.Second

// relay は、接続先ひとつ分の「別タブで開く」中継である。このマシンの
// 127.0.0.1:<localPort> で待ち受け、SSH の接続を通して、接続先の 127.0.0.1:<remotePort>
// のエンジンへ届ける。
type relay struct {
	link      *hostLink
	localPort int
	server    *http.Server
	transport *http.Transport

	mutex      sync.Mutex
	remotePort int
}

func newRelay(link *hostLink) *relay {
	r := &relay{link: link}
	r.transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return link.dialLoopback(ctx, r.currentRemotePort())
		},
		// 圧縮はブラウザと接続先のエンジンのあいだのことで、中継はそのまま運ぶ。
		DisableCompression: true,
	}
	// 終わった接続の上の待機中の HTTP の接続を捨てる。残すと次の要求がそれを使って失敗する。
	link.whenEnded(r.transport.CloseIdleConnections)
	return r
}

func (r *relay) setRemotePort(port int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.remotePort = port
}

func (r *relay) currentRemotePort() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.remotePort
}

// serve は、listener で中継を始める。
func (r *relay) serve(listener net.Listener, port int) {
	r.localPort = port
	r.server = &http.Server{
		Handler:           newRelayHandler(port, r.currentRemotePort, r.transport),
		ReadHeaderTimeout: relayReadHeaderTimeout,
	}
	go func() { _ = r.server.Serve(listener) }()
}

func (r *relay) listening() bool { return r.server != nil }

// close は待ち受けを閉じる。SSH の接続は hostLink が持つので、ここでは閉じない。
func (r *relay) close() {
	if r.server != nil {
		_ = r.server.Close()
	}
	r.transport.CloseIdleConnections()
}

// listenLoopback は、このマシンの 127.0.0.1 の port で待ち受ける。0 なら空いている番号を選ぶ。
func listenLoopback(port int) (net.Listener, int, error) {
	listener, err := net.Listen("tcp", loopbackHost(port))
	if err != nil {
		return nil, 0, err
	}
	_, chosen, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, 0, err
	}
	number, err := strconv.Atoi(chosen)
	if err != nil {
		_ = listener.Close()
		return nil, 0, err
	}
	return listener, number, nil
}
