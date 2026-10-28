package terminalrestore

import (
	"crypto/rand"
	"sync"

	"sshc/internal/randomid"
	"sshc/internal/storage"
)

// Service holds the offer: the sessions the previous engine left open, until
// the user opens them again or declines.
type Service struct {
	store store
	mutex sync.Mutex
	offer []Session
	// offerID names the offer this engine started with, or is empty when there
	// is none. Each engine start draws a new one, so a screen that keeps
	// listing sessions across a restart of the engine still sees a new offer.
	offerID string
}

// NewService reads what the previous engine saved. A record that cannot be
// read is reported and offers nothing; the next stop replaces it.
func NewService(workspace *storage.Workspace) (*Service, error) {
	service := &Service{store: store{workspace: workspace}}
	offer, err := service.store.load()
	service.offer = offer
	if len(offer) > 0 {
		id, idErr := randomid.Token(rand.Reader)
		if idErr != nil {
			return service, idErr
		}
		service.offerID = id
	}
	return service, err
}

// OfferID names the offer waiting to be answered, or is empty when nothing
// waits.
func (s *Service) OfferID() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.offerID
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
	s.offer, s.offerID = nil, ""
	return taken, s.store.save(nil)
}

// Decline ends the offer without opening anything.
func (s *Service) Decline() error {
	_, err := s.Take()
	return err
}

// SaveOnStop records the sessions open now for the next engine. An offer
// still unanswered is kept in front of them, so stopping before answering
// does not lose it. An offered session the user opened again by hand is
// recorded once, as the open one, so the next offer does not open it twice.
func (s *Service) SaveOnStop(open []Session) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	sessions := append(withoutReopened(s.offer, open), open...)
	if len(sessions) > MaxSessions {
		sessions = sessions[:MaxSessions]
	}
	return s.store.save(sessions)
}

// withoutReopened returns the offered sessions that are not among open, each
// open session accounting for one offered session at most: two sessions to a
// host offered and one opened again leave one offered. A session opened by
// hand starts elsewhere and has no name yet, so it is matched by what it
// connects to and the workspace it is in.
func withoutReopened(offered, open []Session) []Session {
	reopened := make(map[Session]int, len(open))
	for _, session := range open {
		reopened[session.connection()]++
	}
	remaining := make([]Session, 0, len(offered))
	for _, session := range offered {
		if key := session.connection(); reopened[key] > 0 {
			reopened[key]--
			continue
		}
		remaining = append(remaining, session)
	}
	return remaining
}

// connection keeps what a session connects to and where it is shown.
func (s Session) connection() Session {
	return Session{Kind: s.Kind, Alias: s.Alias, ShellProfile: s.ShellProfile, Workspace: s.Workspace}
}
