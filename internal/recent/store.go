// Package recent は、この端末で成功した SSH 接続を記録する。
//
// 設定や資格情報とは異なり、この履歴は端末固有の操作状態である。リモート同期へは
// 含めず、表示時には現在の ssh_config を改めて解決する。
package recent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sshc/internal/storage"
)

const (
	SchemaVersion = 1
	PathRelative  = "sshc/recent-connections.json"
	MaxEntries    = 20
	temporaryName = ".recent-connections-"
)

var (
	ErrUnsupportedVersion = errors.New("recent connections were written by a newer version of sshc")
	ErrInvalidDocument    = errors.New("recent connections document is invalid")
)

// Entry は、接続先の不変な入口と、その入口を最後に使った時刻である。
// hostname、user、port は変更されうるため保存しない。
type Entry struct {
	Alias           string `json:"alias"`
	LastConnectedAt string `json:"lastConnectedAt"`
}

type document struct {
	SchemaVersion int     `json:"schemaVersion"`
	Entries       []Entry `json:"entries"`
}

// Store は、ひとつの engine 内の read-modify-write を直列化する。
type Store struct {
	workspace *storage.Workspace
	now       func() time.Time
	mutex     sync.Mutex
}

func NewStore(workspace *storage.Workspace, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{workspace: workspace, now: now}
}

func (s *Store) Path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// List は、新しい接続から順に返す。
func (s *Store) List() ([]Entry, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.load()
}

// Record は、成功した接続を先頭へ移し、上限を超えた古い履歴を捨てる。
func (s *Store) Record(alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" || len(alias) > 64 {
		return ErrInvalidDocument
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	entries, err := s.load()
	if err != nil {
		return err
	}
	updated := make([]Entry, 0, min(len(entries)+1, MaxEntries))
	updated = append(updated, Entry{
		Alias: alias, LastConnectedAt: s.now().UTC().Format(time.RFC3339Nano),
	})
	for _, entry := range entries {
		if entry.Alias == alias || len(updated) == MaxEntries {
			continue
		}
		updated = append(updated, entry)
	}
	return s.write(updated)
}

func (s *Store) load() ([]Entry, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored document
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if stored.SchemaVersion > SchemaVersion {
		return nil, ErrUnsupportedVersion
	}
	if stored.SchemaVersion != SchemaVersion || len(stored.Entries) > MaxEntries {
		return nil, ErrInvalidDocument
	}
	seen := make(map[string]bool, len(stored.Entries))
	entries := append([]Entry(nil), stored.Entries...)
	for _, entry := range entries {
		if strings.TrimSpace(entry.Alias) != entry.Alias || entry.Alias == "" || len(entry.Alias) > 64 || seen[entry.Alias] {
			return nil, ErrInvalidDocument
		}
		seen[entry.Alias] = true
		if _, err := time.Parse(time.RFC3339Nano, entry.LastConnectedAt); err != nil {
			return nil, ErrInvalidDocument
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].LastConnectedAt > entries[j].LastConnectedAt
	})
	return entries, nil
}

func (s *Store) write(entries []Entry) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(document{SchemaVersion: SchemaVersion, Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	directory := s.workspace.StateDir()
	temporary, err := s.workspace.FileSystem().WriteTemp(
		directory, temporaryName, storage.FilePermission, contents,
	)
	if err != nil {
		return err
	}
	defer func() { _ = s.workspace.FileSystem().Remove(temporary) }()
	if err := s.workspace.FileSystem().Rename(temporary, s.Path()); err != nil {
		return err
	}
	return s.workspace.FileSystem().SyncDir(directory)
}
