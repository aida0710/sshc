package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"sshc/internal/storage"
)

const temporaryName = ".workspaces-"

type document struct {
	SchemaVersion int         `json:"schemaVersion"`
	Workspaces    []Workspace `json:"workspaces"`
}

// Store は、ひとつの engine 内の read-modify-write を直列化する。
type Store struct {
	workspace *storage.Workspace
	mutex     sync.Mutex
}

func NewStore(workspace *storage.Workspace) *Store {
	return &Store{workspace: workspace}
}

func (store *Store) Path() string {
	return filepath.Join(store.workspace.Root(), filepath.FromSlash(PathRelative))
}

// List は、最後に更新したものから順に返す。
func (store *Store) List() ([]Workspace, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	workspaces, err := store.load()
	if err != nil {
		return nil, err
	}
	return cloneAll(workspaces), nil
}

func (store *Store) Get(id string) (Workspace, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	workspaces, err := store.load()
	if err != nil {
		return Workspace{}, err
	}
	for _, candidate := range workspaces {
		if candidate.ID == id {
			return clone(candidate), nil
		}
	}
	return Workspace{}, ErrNotFound
}

// Save は、同じ ID の workspace を置換するか、新しい workspace を追加する。
func (store *Store) Save(updated Workspace) error {
	if err := validate(updated); err != nil {
		return err
	}
	created, err := time.Parse(time.RFC3339Nano, updated.CreatedAt)
	if err != nil {
		return ErrInvalidWorkspace
	}
	modified, err := time.Parse(time.RFC3339Nano, updated.UpdatedAt)
	if err != nil || modified.Before(created) {
		return ErrInvalidWorkspace
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()
	workspaces, err := store.load()
	if err != nil {
		return err
	}
	found := false
	for index := range workspaces {
		if workspaces[index].ID == updated.ID {
			workspaces[index] = clone(updated)
			found = true
			break
		}
	}
	if !found {
		if len(workspaces) >= MaxWorkspaces {
			return ErrLimit
		}
		workspaces = append(workspaces, clone(updated))
	}
	sortWorkspaces(workspaces)
	return store.write(workspaces)
}

func (store *Store) Delete(id string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	workspaces, err := store.load()
	if err != nil {
		return err
	}
	for index := range workspaces {
		if workspaces[index].ID != id {
			continue
		}
		workspaces = append(workspaces[:index], workspaces[index+1:]...)
		return store.write(workspaces)
	}
	return ErrNotFound
}

func (store *Store) load() ([]Workspace, error) {
	contents, err := store.workspace.FileSystem().ReadFile(store.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return []Workspace{}, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var stored document
	if err := decoder.Decode(&stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidDocument
	}
	if stored.SchemaVersion > SchemaVersion {
		return nil, ErrUnsupportedSchema
	}
	if stored.SchemaVersion != SchemaVersion || len(stored.Workspaces) > MaxWorkspaces {
		return nil, ErrInvalidDocument
	}
	seen := make(map[string]bool, len(stored.Workspaces))
	for _, candidate := range stored.Workspaces {
		if seen[candidate.ID] || validate(candidate) != nil {
			return nil, ErrInvalidDocument
		}
		seen[candidate.ID] = true
		created, createdErr := time.Parse(time.RFC3339Nano, candidate.CreatedAt)
		updated, updatedErr := time.Parse(time.RFC3339Nano, candidate.UpdatedAt)
		if createdErr != nil || updatedErr != nil || updated.Before(created) {
			return nil, ErrInvalidDocument
		}
	}
	sortWorkspaces(stored.Workspaces)
	return stored.Workspaces, nil
}

func (store *Store) write(workspaces []Workspace) error {
	if err := store.workspace.EnsureDirectory(store.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(document{SchemaVersion: SchemaVersion, Workspaces: workspaces}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	directory := store.workspace.StateDir()
	temporary, err := store.workspace.FileSystem().WriteTemp(directory, temporaryName, storage.FilePermission, contents)
	if err != nil {
		return err
	}
	defer func() { _ = store.workspace.FileSystem().Remove(temporary) }()
	if err := store.workspace.FileSystem().Rename(temporary, store.Path()); err != nil {
		return err
	}
	return store.workspace.FileSystem().SyncDir(directory)
}

func sortWorkspaces(workspaces []Workspace) {
	sort.SliceStable(workspaces, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, workspaces[i].UpdatedAt)
		right, _ := time.Parse(time.RFC3339Nano, workspaces[j].UpdatedAt)
		if left.Equal(right) {
			return workspaces[i].ID < workspaces[j].ID
		}
		return left.After(right)
	})
}

func cloneAll(source []Workspace) []Workspace {
	result := make([]Workspace, len(source))
	for index := range source {
		result[index] = clone(source[index])
	}
	return result
}
