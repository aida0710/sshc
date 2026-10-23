package sshclient

import (
	"io"
	"sync"
)

// observedOutput は、標準出力と標準エラーを 1 本の Command.Observe へ渡す。
//
// SSH のチャンネルは、標準出力と標準エラーを別の goroutine で書く。ひとつの mutex で
// 順に渡すので、受け取る側は 2 本が混ざる順を保ったまま、競合なく読める。
type observedOutput struct {
	mutex    sync.Mutex
	observer io.Writer
}

func newObservedOutput(observer io.Writer) *observedOutput {
	return &observedOutput{observer: observer}
}

// tee は、captured へ書き、同じ内容を observer へも渡す writer を返す。
func (o *observedOutput) tee(captured io.Writer) io.Writer {
	return observedStream{captured: captured, output: o}
}

func (o *observedOutput) write(p []byte) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	// 見せる側の失敗で、コマンドの結果を変えない。
	_, _ = o.observer.Write(p)
}

// observedStream は、標準出力か標準エラーの片方である。
type observedStream struct {
	captured io.Writer
	output   *observedOutput
}

func (s observedStream) Write(p []byte) (int, error) {
	s.output.write(p)
	return s.captured.Write(p)
}
