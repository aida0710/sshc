package terminal

// ControlState は、自動化に向いた、はっきりしたライフサイクルの状態である。使えるかどうかを、ターミナルの
// 文字から推し量ることは無い。
type ControlState string

const (
	ControlConnecting   ControlState = "connecting"
	ControlConnected    ControlState = "connected"
	ControlReconnecting ControlState = "reconnecting"
	ControlExited       ControlState = "exited"
)

// ControlSnapshot は、スクロールバックの読み1つを、同じロックの中で見た process の世代と、ライフサイクルの
// 状態に結び付ける。
type ControlSnapshot struct {
	Generation uint64
	State      ControlState
	Read       RingRead
}

// ReadControl は、長さに上限のあるスクロールバックと、機械が読める状態を返す。bool が false になるのは、
// cursor がこのセッションの今の出力より先を指すときだけである。
func (s *Session) ReadControl(cursor uint64, limit int) (ControlSnapshot, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	read, ok := s.buffer.ReadFrom(cursor, limit)
	if !ok {
		return ControlSnapshot{}, false
	}
	return ControlSnapshot{Generation: s.generation, State: s.controlStateLocked(), Read: read}, true
}

func (s *Session) controlStateLocked() ControlState {
	switch s.state {
	case StateConnecting:
		return ControlConnecting
	case StateReconnecting:
		return ControlReconnecting
	case StateExited:
		return ControlExited
	}
	return ControlConnected
}
