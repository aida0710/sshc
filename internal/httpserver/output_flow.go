package httpserver

import (
	"context"
	"sync"
)

const (
	// maxOutputFrame は、出力のフレームひとつの上限である。溜まった出力はまとめて送るが、
	// ブラウザが残りを受け取るあいだに描き始められる大きさに分ける。
	maxOutputFrame = 64 << 10
	// unacknowledgedOutputLimit は、ブラウザがまだ解析し終えていない出力の上限である。
	// xterm.js は 1 秒に数十 MB を解析するので、これは 1 秒に満たない仕事である。
	// 打った Ctrl+C の反応が、それより多く溜まった出力の後ろで待たされない。
	unacknowledgedOutputLimit = 2 << 20
)

// outputFlow は、ブラウザが追いつくまで出力を待たせる。ブラウザが xterm.js で
// 解析し終えたバイト数を {"ack": n} で知らせてきたら、送ったバイトとの差を
// unacknowledgedOutputLimit までに抑える。一度も知らせてこないブラウザは待たせない。
// 実行先のエンジンへは、版の違うエンジンが出した画面も繋いでくるためである。
// 待つあいだの出力は再生バッファに入り、そこから押し出された分は飛ばす
// （terminal.Stream）。
type outputFlow struct {
	mutex         sync.Mutex
	sent          uint64
	acknowledged  uint64
	acknowledging bool
	changed       chan struct{}
}

func newOutputFlow() *outputFlow {
	return &outputFlow{changed: make(chan struct{}, 1)}
}

// wrote は、ブラウザへ送った出力のバイト数を足す。
func (f *outputFlow) wrote(bytes int) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.sent += uint64(bytes)
}

// acknowledge は、ブラウザが解析し終えたと知らせたバイト数を受け取る。送った分を
// 超える値と、前より小さい値は信じない。
func (f *outputFlow) acknowledge(parsed uint64) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.acknowledging = true
	f.acknowledged = max(f.acknowledged, min(parsed, f.sent))
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

// waitForRoom は、送ってよくなるまで待つ。ctx が終わったら false を返す。
func (f *outputFlow) waitForRoom(ctx context.Context) bool {
	for {
		f.mutex.Lock()
		room := !f.acknowledging || f.sent-f.acknowledged < unacknowledgedOutputLimit
		f.mutex.Unlock()
		if room {
			return true
		}
		select {
		case <-f.changed:
		case <-ctx.Done():
			return false
		}
	}
}
