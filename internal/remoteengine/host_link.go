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

// hostLink は、接続先ひとつへの SSH の接続を持つ。別タブの中継とターミナルの取り次ぎが
// 同じ接続を使い、接続先ひとつに SSH の接続を二本開かない。
type hostLink struct {
	connect func(context.Context) (Link, error)

	mutex sync.Mutex
	link  Link
	// ended は、接続が終わったときに呼ぶ。待機中の HTTP の接続を捨てるために使う。
	ended []func()
}

func newHostLink(connect func(context.Context) (Link, error)) *hostLink {
	return &hostLink{connect: connect}
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
func (h *hostLink) live(ctx context.Context) (Link, error) {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.link != nil {
		return h.link, nil
	}
	link, err := h.connect(ctx)
	if err != nil {
		return nil, &ConnectError{Err: err}
	}
	h.link = link
	go h.forgetWhenEnded(link)
	return link, nil
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

func (h *hostLink) close() {
	h.mutex.Lock()
	link := h.link
	h.link = nil
	h.mutex.Unlock()
	if link != nil {
		_ = link.Close()
	}
}
