package sshclient

import (
	"io"
	"sync"
)

// shellOutputBacklog は、見張りより先にシェルが書いたもののうち、見張りに渡す量である。接続時の手順が
// 待つのはプロンプトのような直前に出る短い文字列なので、最後の 16 KiB で足りる。
const shellOutputBacklog = 16 << 10

// shellOutput は、リモートのシェルが書いたものだけを、端末へ出す道とは別に見張りへ渡す。端末へ出る
// 道には、認証の問いや [sshc] の行も通る（Session.writer）。接続時の手順はここだけを見るので、
// 認証の問いに答えることはない（terminal.ShellOutput）。
type shellOutput struct {
	mutex    sync.Mutex
	backlog  []byte
	watchers map[int]func([]byte)
	next     int
}

// tee は、destination へ書くものを、見張りにも渡す writer を返す。
func (o *shellOutput) tee(destination io.Writer) io.Writer {
	return shellOutputWriter{output: o, destination: destination}
}

// observe は、シェルが書いた p を覚え、見張りへ渡す。見張りは書いた順に受け取るよう、錠を持ったまま呼ぶ。
func (o *shellOutput) observe(p []byte) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.backlog = append(o.backlog, p...)
	if excess := len(o.backlog) - shellOutputBacklog; excess > 0 {
		o.backlog = append(o.backlog[:0], o.backlog[excess:]...)
	}
	for _, seen := range o.watchers {
		seen(p)
	}
}

// watch は、seen に、これまでに書いたものの終わり（shellOutputBacklog まで）を渡し、そのあと書くものを
// 書いた順に渡す。stop を呼ぶまで続ける。
func (o *shellOutput) watch(seen func([]byte)) (stop func()) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if len(o.backlog) > 0 {
		seen(append([]byte(nil), o.backlog...))
	}
	if o.watchers == nil {
		o.watchers = make(map[int]func([]byte))
	}
	id := o.next
	o.next++
	o.watchers[id] = seen
	return func() {
		o.mutex.Lock()
		defer o.mutex.Unlock()
		delete(o.watchers, id)
	}
}

type shellOutputWriter struct {
	output      *shellOutput
	destination io.Writer
}

func (w shellOutputWriter) Write(p []byte) (int, error) {
	w.output.observe(p)
	return w.destination.Write(p)
}

// WatchShellOutput は terminal.ShellOutput である。
func (s *Session) WatchShellOutput(seen func([]byte)) (stop func()) {
	return s.shellOutput.watch(seen)
}
