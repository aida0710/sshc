package browserauth

import "time"

// Every grace token points at the current capability of its registration.
// Updating a family never extends the original grace deadlines of older tabs.
func (s *Store) rotateCachedTokens(previousHash, token string, until time.Time) {
	for hash, recent := range s.rotations {
		if hashToken(recent.token) == previousHash {
			recent.token = token
			s.rotations[hash] = recent
		}
	}
	s.rotations[previousHash] = rotation{token: token, until: until}
}

// A cached capability cannot outlive the registration on disk, whether it was
// signed out, expired, evicted, or changed by another store instance.
func (s *Store) pruneRotations(registrations []registration) {
	now := s.now()
	for hash, recent := range s.rotations {
		if !now.Before(recent.until) || indexOf(registrations, hashToken(recent.token), currentHash) < 0 {
			delete(s.rotations, hash)
		}
	}
}
