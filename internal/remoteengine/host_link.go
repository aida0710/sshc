package remoteengine

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// Link は、接続先への認証済みの SSH の接続ひとつである。
type Link interface {
	// DialContext は、接続先から見た address へ TCP を開く。
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	// Run は、接続先でコマンドをひとつ走らせる。接続は開いたままにする。
	Run(ctx context.Context, request RunRequest) (RunResult, error)
	// Ping は、keepalive を1回送り、limit のうちに SSH サーバーが答えるかを確かめる。
	// 答えなければその理由を返す。
	Ping(limit time.Duration) error
	// Wait は、接続が終わるまで待つ。
	Wait() error
	Close() error
}

// RunRequest は、Link.Run が接続先で走らせるコマンドひとつである。
type RunRequest struct {
	Command string
	// Limit は、コマンドが終わるまでの上限である。
	Limit time.Duration
	// Observe は、標準出力と標準エラーを届いた順に受け取る。nil なら渡さない。
	Observe io.Writer
}

// RunResult は、Link.Run が走らせたコマンドの結果である。
type RunResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// linkAttemptLimit は、接続先への SSH の接続ひとつを試す上限である。sshclient は認証に入ると
// ConnectTimeout の時計を止めるので、認証に答えない相手との試みはここで終える。VPN の経路の
// 起動（スマートフォンでの承認の待ちが2分）と、認証の待ち（OpenSSH の LoginGraceTime の既定が
// 2分）を足しても収まる長さにする。
const linkAttemptLimit = 5 * time.Minute

// hostLink は、接続先ひとつへの SSH の接続を持つ。別タブの中継とターミナルの取り次ぎが
// 同じ接続を使い、接続先ひとつに SSH の接続を二本開かない。
type hostLink struct {
	connect func(context.Context) (Link, error)
	// connecting は、同時に来た要求に、ひとつの接続の試みを待たせる。
	connecting sharedAttempt[Link]
	// attemptLimit は、接続の試みひとつの上限（linkAttemptLimit）である。試験だけが短くする。
	attemptLimit time.Duration

	mutex sync.Mutex
	link  Link
	// closed は、close のあとで、終わった試みの接続を持たずに閉じるために使う。
	closed bool
	// ended は、接続が終わったときに呼ぶ。待機中の HTTP の接続を捨てるために使う。
	ended []func()
}

func newHostLink(connect func(context.Context) (Link, error), attemptLimit time.Duration) *hostLink {
	return &hostLink{connect: connect, attemptLimit: attemptLimit}
}

// whenEnded は、接続が終わるたびに呼ぶ処理を足す。
func (h *hostLink) whenEnded(action func()) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.ended = append(h.ended, action)
}

// live は、生きている SSH の接続を返す。切れていれば、一度だけ繋ぎ直す。
//
// 繋ぎ直せなければ、そのまま失敗を返す。回数を重ねて試すと、繋がらない理由
// （ノートパソコンのスリープではなく、鍵や接続先の変更）が見えなくなる。
//
// 接続はロックの外で試す。接続先が応答しないと、試みは attemptLimit まで続く。
// そのあいだに来た要求は、同じ試みの結果を待つ。
func (h *hostLink) live(ctx context.Context) (Link, error) {
	h.mutex.Lock()
	link, closed := h.link, h.closed
	h.mutex.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if link != nil {
		return link, nil
	}
	return h.connecting.do(ctx, h.attemptLimit, h.connectOnce)
}

func (h *hostLink) connectOnce(ctx context.Context) (Link, error) {
	link, err := h.connect(ctx)
	if err != nil {
		return nil, &ConnectError{Err: err}
	}
	h.mutex.Lock()
	if h.closed {
		h.mutex.Unlock()
		_ = link.Close()
		return nil, ErrClosed
	}
	h.link = link
	h.mutex.Unlock()
	go h.forgetWhenEnded(link)
	return link, nil
}

// linkResponseLimit は、SSH の接続が keepalive に答えるのを待つ上限である。答えは SSH の
// 往復1回で返るので、遅い回線でも数秒で足りる。
const linkResponseLimit = 5 * time.Second

// resetIfSilent は、SSH の接続が keepalive に答えなければ閉じ、次の要求で繋ぎ直させる。
// SSH の接続は、相手が黙って消えると keepalive が気づくまで（数十秒から2分ほど）生きている
// ように見える。
//
// 答えるなら閉じない。止まったのは接続先のエンジンで、同じ接続の上で走っている操作
// （更新・再起動の出力）や別タブの中継、ほかのセッションの入出力まで巻き添えで切らないため
// である。
func (h *hostLink) resetIfSilent() {
	h.mutex.Lock()
	link := h.link
	h.mutex.Unlock()
	if link == nil || link.Ping(linkResponseLimit) == nil {
		return
	}
	h.mutex.Lock()
	if h.link == link {
		h.link = nil
	}
	h.mutex.Unlock()
	_ = link.Close()
}

// forgetWhenEnded は、接続が終わったら、次の要求で繋ぎ直すよう手放す。
func (h *hostLink) forgetWhenEnded(link Link) {
	_ = link.Wait()
	h.mutex.Lock()
	if h.link == link {
		h.link = nil
	}
	ended := append([]func(){}, h.ended...)
	h.mutex.Unlock()
	for _, action := range ended {
		action()
	}
}

// dialLoopback は、接続先の 127.0.0.1:port へ TCP を開く。
func (h *hostLink) dialLoopback(ctx context.Context, port int) (net.Conn, error) {
	link, err := h.live(ctx)
	if err != nil {
		return nil, err
	}
	return link.DialContext(ctx, "tcp", loopbackHost(port))
}

// close は接続を閉じ、走っている接続の試みも取り消す。以後の要求は ErrClosed で断る。
func (h *hostLink) close() {
	h.mutex.Lock()
	link := h.link
	h.link = nil
	h.closed = true
	h.mutex.Unlock()
	h.connecting.abandon()
	if link != nil {
		_ = link.Close()
	}
}
