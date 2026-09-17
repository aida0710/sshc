package terminal

// ControlState is an explicit lifecycle observation suitable for automation.
// It never derives readiness from terminal text.
type ControlState string

const (
	ControlConnecting   ControlState = "connecting"
	ControlConnected    ControlState = "connected"
	ControlReconnecting ControlState = "reconnecting"
	ControlExited       ControlState = "exited"
)

// ControlSnapshot binds one scrollback read to the exact process generation
// and explicit lifecycle state observed under the same lock.
type ControlSnapshot struct {
	Generation uint64
	State      ControlState
	Read       RingRead
}

// ReadControl returns bounded scrollback and machine-readable state. The bool
// is false only when cursor points beyond this session's current output.
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
