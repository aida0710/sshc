package terminal

// TransportCheckPending は、セッションが、接続の確認への答えをまだ受け取っていないかを返す。
func (s *Session) TransportCheckPending() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.reconnection.transportCheckPending
}
