package terminal

// TransportCheckPending says whether the session has not heard an answer to its check of the connection yet.
func (s *Session) TransportCheckPending() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.reconnection.transportCheckPending
}
