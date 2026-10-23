package terminal

// ForegroundDirectory は、このセッションで前面にいるプログラムの作業ディレクトリを
// 返す。接続していないとき ErrNotConnected、Process が答えられないとき
// ErrDirectoryUnavailable を返す。
func (s *Session) ForegroundDirectory() (string, error) {
	s.mutex.Lock()
	if s.stoppedLocked() || s.exited != nil || s.process == nil || s.state != StateConnected {
		s.mutex.Unlock()
		return "", ErrNotConnected
	}
	reporter, ok := s.process.(DirectoryReporter)
	s.mutex.Unlock()
	if !ok {
		return "", ErrDirectoryUnavailable
	}
	return reporter.ForegroundDirectory()
}
