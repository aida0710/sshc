package remoteengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"sshc/internal/storage"
	"sshc/internal/validate"
)

const (
	storeSchemaVersion = 1
	// PathRelative は、接続先のエンジンについて覚えておくファイルである。実行先に足した
	// エンジンの一覧と、接続先ごとに「別タブで開く」中継が待ち受けたポートを持つ。
	// ブラウザの保存（設定、途中のダウンロード）はポートごとに分かれるので、開くたびに
	// ポートが変わると、それが毎回空に戻る。このマシンだけのものなので、同期では運ばない。
	PathRelative       = "sshc/remote-engines.json"
	storeTemporaryName = ".remote-engines-"
)

// ErrInvalidStore は、覚えておくファイルが読めない形であることを表す。
var ErrInvalidStore = errors.New("the saved remote engines are invalid")

type storeDocument struct {
	SchemaVersion int `json:"schemaVersion"`
	// Engines は、実行先に足したエンジンの alias を、足した順に並べる。
	Engines []string       `json:"engines"`
	Ports   map[string]int `json:"ports"`
}

// Store は、実行先に足したエンジンと、接続先の alias ごとに中継が待ち受けたこのマシンの
// ポートを覚える。
type Store struct {
	workspace *storage.Workspace
	mutex     sync.Mutex
}

func NewStore(workspace *storage.Workspace) *Store {
	return &Store{workspace: workspace}
}

func (s *Store) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// Port は、alias の中継が前に待ち受けたポートを返す。覚えていなければ false。
func (s *Store) Port(alias string) (int, bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return 0, false, err
	}
	port, found := stored.Ports[alias]
	return port, found, nil
}

// Remember は、alias の中継が待ち受けたポートを覚える。
func (s *Store) Remember(alias string, port int) error {
	if err := validate.Alias(alias); err != nil || !validPort(port) {
		return ErrInvalidStore
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return err
	}
	if stored.Ports[alias] == port {
		return nil
	}
	stored.Ports[alias] = port
	return s.write(stored)
}

// Engines は、実行先に足したエンジンの alias を、足した順に返す。
func (s *Store) Engines() ([]string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return nil, err
	}
	return stored.Engines, nil
}

// AddEngine は、実行先にエンジンを足す。足してあれば何もしない。
func (s *Store) AddEngine(alias string) error {
	if err := validate.Alias(alias); err != nil {
		return ErrInvalidStore
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return err
	}
	if slices.Contains(stored.Engines, alias) {
		return nil
	}
	stored.Engines = append(stored.Engines, alias)
	return s.write(stored)
}

// RemoveEngine は、実行先からエンジンを外す。中継のポートは残す。
func (s *Store) RemoveEngine(alias string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return err
	}
	index := slices.Index(stored.Engines, alias)
	if index < 0 {
		return nil
	}
	stored.Engines = slices.Delete(stored.Engines, index, index+1)
	return s.write(stored)
}

func (s *Store) load() (storeDocument, error) {
	empty := storeDocument{SchemaVersion: storeSchemaVersion, Engines: []string{}, Ports: map[string]int{}}
	contents, err := s.workspace.FileSystem().ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return storeDocument{}, err
	}
	var stored storeDocument
	if err := json.Unmarshal(contents, &stored); err != nil {
		return storeDocument{}, fmt.Errorf("%w: %v", ErrInvalidStore, err)
	}
	if stored.SchemaVersion != storeSchemaVersion {
		return storeDocument{}, ErrInvalidStore
	}
	for alias, port := range stored.Ports {
		if validate.Alias(alias) != nil || !validPort(port) {
			return storeDocument{}, ErrInvalidStore
		}
		empty.Ports[alias] = port
	}
	for _, alias := range stored.Engines {
		if validate.Alias(alias) != nil || slices.Contains(empty.Engines, alias) {
			return storeDocument{}, ErrInvalidStore
		}
		empty.Engines = append(empty.Engines, alias)
	}
	return empty, nil
}

func (s *Store) write(stored storeDocument) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.path(), storeTemporaryName, storage.FilePermission, contents)
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}
