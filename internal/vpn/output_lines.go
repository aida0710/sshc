package vpn

import (
	"bytes"
	"strings"
	"sync"
)

// 実行中のプログラムが書く出力を、書かれるたびに行へ分けて渡す。
//
// docker build は数分かかり、そのあいだの出力を接続ログへ流す。終わってから
// まとめて渡すと、見ている人には止まっているように見える。

// maxPendingLineBytes は、改行の来ない出力を1行として渡すまでに溜める上限である。
// 改行の無いまま長く続く出力で、メモリを使い続けない。
const maxPendingLineBytes = 64 << 10

// lineSplitter は、標準出力と標準エラーのそれぞれを行に分けて emit へ渡す。
// 2つの流れは別々の goroutine から書かれるので、emit は1つずつ呼ぶ。
type lineSplitter struct {
	mutex sync.Mutex
	emit  func(line string)
}

// lineStream は、1つの流れ（標準出力か標準エラー）の、まだ改行の来ていない部分を持つ。
type lineStream struct {
	splitter *lineSplitter
	pending  []byte
}

func (splitter *lineSplitter) stream() *lineStream {
	return &lineStream{splitter: splitter}
}

func (stream *lineStream) Write(chunk []byte) (int, error) {
	stream.splitter.mutex.Lock()
	defer stream.splitter.mutex.Unlock()
	stream.pending = append(stream.pending, chunk...)
	for {
		end := bytes.IndexByte(stream.pending, '\n')
		if end < 0 {
			break
		}
		stream.emitLine(stream.pending[:end])
		stream.pending = stream.pending[end+1:]
	}
	if len(stream.pending) > maxPendingLineBytes {
		stream.emitLine(stream.pending)
		stream.pending = nil
	}
	return len(chunk), nil
}

// flush は、最後に改行の無いまま残った部分を1行として渡す。プログラムが終わってから呼ぶ。
func (stream *lineStream) flush() {
	stream.splitter.mutex.Lock()
	defer stream.splitter.mutex.Unlock()
	stream.emitLine(stream.pending)
	stream.pending = nil
}

// emitLine は、空でない行を渡す。mutex を握って呼ぶこと。
func (stream *lineStream) emitLine(line []byte) {
	if text := strings.TrimRight(string(line), "\r"); strings.TrimSpace(text) != "" {
		stream.splitter.emit(text)
	}
}
