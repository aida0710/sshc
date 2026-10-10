package terminal

// Forwarding can wait on a server reply. Keep that wait outside the session
// lock so closing the process can cancel it and session views remain usable.
func (s *Session) StartForward(kind, listenPort, destination string) (Forward, error) {
	controller, generation, err := s.forwardController()
	if err != nil {
		return Forward{}, err
	}
	forward, err := controller.StartForward(kind, listenPort, destination)
	if !s.isForwardGenerationConnected(generation) {
		if err == nil && forward.ID != "" {
			_ = controller.StopForward(forward.ID)
		}
		return Forward{}, ErrNotConnected
	}
	return forward, err
}

func (s *Session) StopForward(id string) error {
	controller, generation, err := s.forwardController()
	if err != nil {
		return err
	}
	err = controller.StopForward(id)
	if !s.isForwardGenerationConnected(generation) {
		return ErrNotConnected
	}
	return err
}

func (s *Session) forwardController() (ForwardController, uint64, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.stoppedLocked() || s.exited != nil || s.process == nil || s.state != StateConnected {
		return nil, 0, ErrNotConnected
	}
	controller, ok := s.process.(ForwardController)
	if !ok {
		return nil, 0, ErrForwardUnavailable
	}
	return controller, s.generation, nil
}

func (s *Session) isForwardGenerationConnected(generation uint64) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.generation == generation && !s.stoppedLocked() && s.exited == nil && s.state == StateConnected
}
