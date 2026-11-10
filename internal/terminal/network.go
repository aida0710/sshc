package terminal

import "sync"

// NetworkWatch は、このマシンの回線について OS が言うことを持つ。分かるマシン（Android のアプリ。
// ConnectivityManager から）のためのものである。分からないマシンは持たない。その場合は、回線が無くて失敗した
// 接続の試みが、そのことを伝える（ProblemNoNetwork）。
//
// nil の *NetworkWatch は、何も言わないマシンである。
type NetworkWatch struct {
	mutex       sync.Mutex
	unavailable bool
	changed     chan struct{}
}

// NewNetworkWatch は、別のことを言われるまで、回線があるものとして扱う見張りを返す。
func NewNetworkWatch() *NetworkWatch {
	return &NetworkWatch{changed: make(chan struct{})}
}

// SetAvailable は、OS が言ったことを記録する。回線があると言われたら、すでにあったとしても、待っている側を
// 起こす。新しい回線かもしれず（Wi-Fi からモバイル回線へ）、次の番を待っている試みは今すぐ進める。
func (w *NetworkWatch) SetAvailable(available bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if !available && w.unavailable {
		return
	}
	w.unavailable = !available
	close(w.changed)
	w.changed = make(chan struct{})
}

// State は、回線があるかと、OS が次に何かを言ったときに閉じるチャネルを返す。nil の見張りのチャネルは
// 閉じない。
func (w *NetworkWatch) State() (available bool, changed <-chan struct{}) {
	if w == nil {
		return true, nil
	}
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return !w.unavailable, w.changed
}
