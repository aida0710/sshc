package snippets

import (
	"sshc/internal/storage"
	"sshc/internal/validate"
)

// WithStartupRename joins a config/vault transaction. The caller holds the
// vault generation lock, just as for TravelDocument; taking WithMutation again
// would invert that lock order. The encrypted baseline prevents lost updates.
func (s *Store) WithStartupRename(from, to string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error) {
	if err := validate.Alias(to); err != nil {
		return storage.Result{}, ErrInvalidTarget
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if from == to {
		return commit(nil)
	}
	loaded, err := s.readDocument()
	if err != nil {
		return storage.Result{}, err
	}
	if !renameStartup(&loaded.library, from, to) {
		return commit(nil)
	}
	contents, err := encodeDocument(loaded.library)
	if err != nil {
		return storage.Result{}, err
	}
	sealed, err := s.sealDocument(contents)
	if err != nil {
		return storage.Result{}, err
	}
	return commit(&storage.Change{
		Path: s.Path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(loaded.contents)},
	})
}

// The source's opt-in follows it; a stale destination cannot attach automation
// to a host which never opted in. Other snippets and their inputs are retained.
func renameStartup(library *Library, from, to string) bool {
	kept := make([]Startup, 0, len(library.Startup))
	changed := false
	for _, startup := range library.Startup {
		switch startup.Alias {
		case from:
			startup.Alias = to
			kept = append(kept, startup)
			changed = true
		case to:
			changed = true
		default:
			kept = append(kept, startup)
		}
	}
	library.Startup = kept
	return changed
}
