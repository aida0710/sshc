package terminalrestore

import (
	"sync"

	"sshc/internal/storage"
)

// Service holds the offer: the sessions the previous engine left open, until
// the user opens them again or declines.
type Service struct {
	store store
	mutex sync.Mutex
	offer []Session
}

// NewService reads what the previous engine saved. A record that cannot be
// read is reported and offers nothing; the next stop replaces it.
func NewService(workspace *storage.Workspace) (*Service, error) {
	service := &Service{store: store{workspace: workspace}}
	offer, err := service.store.load()
	service.offer = offer
	return service, err
}

// Offer returns the sessions waiting to be opened again.
func (s *Service) Offer() []Session {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]Session(nil), s.offer...)
}

// Take returns the offer and ends it. The caller opens the sessions.
func (s *Service) Take() ([]Session, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	taken := s.offer
	s.offer = nil
	return taken, s.store.save(nil)
}

// Decline ends the offer without opening anything.
func (s *Service) Decline() error {
	_, err := s.Take()
	return err
}

// SaveOnStop records the sessions open now for the next engine. An offer
// still unanswered is kept in front of them, so stopping before answering
// does not lose it.
func (s *Service) SaveOnStop(open []Session) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	sessions := append(append([]Session(nil), s.offer...), open...)
	if len(sessions) > MaxSessions {
		sessions = sessions[:MaxSessions]
	}
	return s.store.save(sessions)
}
