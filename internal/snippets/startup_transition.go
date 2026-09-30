package snippets

import (
	"slices"

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
	if from == to {
		return commit(nil)
	}
	return s.withStartupTransition(func(library *Library) bool {
		return renameStartup(library, from, to)
	}, commit)
}

// WithStartupRemoval joins a config/vault transaction exactly like
// WithStartupRename, for a save that removes the Host declarations of aliases.
// A host later created under the same alias never opted in, so its startup
// assignment must not survive the removal.
func (s *Store) WithStartupRemoval(aliases []string, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error) {
	if len(aliases) == 0 {
		return commit(nil)
	}
	return s.withStartupTransition(func(library *Library) bool {
		return removeStartup(library, aliases)
	}, commit)
}

// WithStartupRebind joins a VPN profile rename or removal exactly like
// WithStartupRename. An assignment's binding includes the name of the VPN
// profile its host goes through, so renaming the profile moves the binding and
// removing it stops the assignment. rebind returns the binding an assignment
// should hold afterwards and true, an empty binding to stop the assignment, or
// false to leave it alone.
func (s *Store) WithStartupRebind(
	rebind func(alias, binding string) (string, bool),
	commit func(*storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.withStartupTransition(func(library *Library) bool {
		return rebindStartup(library, rebind)
	}, commit)
}

// withStartupTransition hands commit the sealed document with transition
// applied, or nil when transition changed nothing. The store stays locked until
// commit returns so no other startup change interleaves.
func (s *Store) withStartupTransition(transition func(*Library) bool, commit func(*storage.Change) (storage.Result, error)) (storage.Result, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	loaded, err := s.readDocument()
	if err != nil {
		return storage.Result{}, err
	}
	if !transition(&loaded.library) {
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

func rebindStartup(library *Library, rebind func(alias, binding string) (string, bool)) bool {
	changed := false
	for index, startup := range library.Startup {
		rebound, ok := rebind(startup.Alias, startup.Binding)
		if !ok || rebound == startup.Binding {
			continue
		}
		library.Startup[index].Binding = rebound
		changed = true
	}
	return changed
}

func removeStartup(library *Library, aliases []string) bool {
	kept := make([]Startup, 0, len(library.Startup))
	for _, startup := range library.Startup {
		if !slices.Contains(aliases, startup.Alias) {
			kept = append(kept, startup)
		}
	}
	changed := len(kept) != len(library.Startup)
	library.Startup = kept
	return changed
}
