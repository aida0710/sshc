package remoteengine

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Link は、接続先への認証済みの SSH の接続ひとつである。
type Link interface {
	// DialContext は、接続先から見た address へ TCP を開く。
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	// Run は、接続先でコマンドをひとつ走らせる。接続は開いたままにする。
	Run(ctx context.Context, command string, limit time.Duration) (RunResult, error)
	// Wait は、接続が終わるまで待つ。
	Wait() error
	Close() error
}

// RunResult は、Link.Run が走らせたコマンドの結果である。
type RunResult struct {
	Stdout   []byte
	ExitCode int
}

// relayReadHeaderTimeout は、中継がブラウザの要求のヘッダーを読み終えるまでの上限である。
// 待ち受けは 127.0.0.1 だけだが、ヘッダーを送らずに居座る接続を残さない。
const relayReadHeaderTimeout = 10 * time.Second

// relay は、接続先ひとつ分の中継である。このマシンの 127.0.0.1:<localPort> で待ち受け、
// SSH の接続を通して、接続先の 127.0.0.1:<remotePort> のエンジンへ届ける。
type relay struct {
	connect   func(context.Context) (Link, error)
	localPort int
	server    *http.Server
	transport *http.Transport

	mutex      sync.Mutex
	link       Link
	remotePort int
}

func newRelay(connect func(context.Context) (Link, error)) *relay {
	r := &relay{connect: connect}
	r.transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return r.dial(ctx)
		},
		// 圧縮はブラウザと接続先のエンジンのあいだのことで、中継はそのまま運ぶ。
		DisableCompression: true,
	}
	return r
}

// liveLink は、生きている SSH の接続を返す。切れていれば、一度だけ繋ぎ直す。
//
// 繋ぎ直せなければ、そのまま失敗を返す。回数を重ねて試すと、繋がらない理由
// （ノートパソコンのスリープではなく、鍵や接続先の変更）が見えなくなる。
func (r *relay) liveLink(ctx context.Context) (Link, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.link != nil {
		return r.link, nil
	}
	link, err := r.connect(ctx)
	if err != nil {
		return nil, &ConnectError{Err: err}
	}
	r.link = link
	go r.forgetWhenEnded(link)
	return link, nil
}

// forgetWhenEnded は、接続が終わったら、次の要求で繋ぎ直すよう手放す。
// 終わった接続の上の待機中の HTTP の接続も捨てる。残すと次の要求がそれを使って失敗する。
func (r *relay) forgetWhenEnded(link Link) {
	_ = link.Wait()
	r.mutex.Lock()
	if r.link == link {
		r.link = nil
	}
	r.mutex.Unlock()
	r.transport.CloseIdleConnections()
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

// dial は、接続先のエンジンへの TCP をひとつ開く。
func (r *relay) dial(ctx context.Context) (net.Conn, error) {
	link, err := r.liveLink(ctx)
	if err != nil {
		return nil, err
	}
	return link.DialContext(ctx, "tcp", loopbackHost(r.currentRemotePort()))
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

// close は、待ち受けと SSH の接続を閉じる。中継中のターミナルの接続は、SSH の接続と
// 一緒に終わる。
func (r *relay) close() {
	if r.server != nil {
		_ = r.server.Close()
	}
	r.mutex.Lock()
	link := r.link
	r.link = nil
	r.mutex.Unlock()
	if link != nil {
		_ = link.Close()
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
