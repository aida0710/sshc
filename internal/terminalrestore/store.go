package terminalrestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"sshc/internal/storage"
)

const temporaryName = ".terminal-sessions-"

type document struct {
	SchemaVersion int       `json:"schemaVersion"`
	Sessions      []Session `json:"sessions"`
}

// store reads and writes terminal-sessions.json. Service serializes the calls.
type store struct {
	workspace *storage.Workspace
}

func (s store) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// load returns the saved sessions, or none when nothing was saved.
func (s store) load() ([]Session, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored document
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if stored.SchemaVersion != SchemaVersion || len(stored.Sessions) > MaxSessions {
		return nil, ErrInvalidDocument
	}
	for _, session := range stored.Sessions {
		if !session.valid() {
			return nil, ErrInvalidDocument
		}
	}
	return stored.Sessions, nil
}

// save replaces the record. With no sessions it removes the file, so a later
// start offers nothing.
func (s store) save(sessions []Session) error {
	if len(sessions) == 0 {
		err := s.workspace.FileSystem().Remove(s.path())
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(document{SchemaVersion: SchemaVersion, Sessions: sessions}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.path(), temporaryName, storage.FilePermission, contents)
}
