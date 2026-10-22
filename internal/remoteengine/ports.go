package remoteengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"

	"sshc/internal/storage"
	"sshc/internal/validate"
)

const (
	portsSchemaVersion = 1
	// PortsPathRelative は、接続先ごとに中継が待ち受けたポートを覚えておくファイルである。
	// このマシンのブラウザの保存（設定、途中のダウンロード）はポートごとに分かれる。
	// 開くたびにポートが変わると、それが毎回空に戻る。このマシンだけのものなので、
	// 同期では運ばない。
	PortsPathRelative  = "sshc/remote-engines.json"
	portsTemporaryName = ".remote-engines-"
)

// ErrInvalidPorts は、ポートを覚えておくファイルが読めない形であることを表す。
var ErrInvalidPorts = errors.New("the saved remote engine ports are invalid")

type portsDocument struct {
	SchemaVersion int            `json:"schemaVersion"`
	Ports         map[string]int `json:"ports"`
}

// PortStore は、接続先の alias ごとに、中継が待ち受けたこのマシンのポートを覚える。
type PortStore struct {
	workspace *storage.Workspace
	mutex     sync.Mutex
}

func NewPortStore(workspace *storage.Workspace) *PortStore {
	return &PortStore{workspace: workspace}
}

func (s *PortStore) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PortsPathRelative))
}

// Port は、alias の中継が前に待ち受けたポートを返す。覚えていなければ false。
func (s *PortStore) Port(alias string) (int, bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ports, err := s.load()
	if err != nil {
		return 0, false, err
	}
	port, found := ports[alias]
	return port, found, nil
}

// Remember は、alias の中継が待ち受けたポートを覚える。
func (s *PortStore) Remember(alias string, port int) error {
	if err := validate.Alias(alias); err != nil || !validPort(port) {
		return ErrInvalidPorts
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	ports, err := s.load()
	if err != nil {
		return err
	}
	if ports[alias] == port {
		return nil
	}
	ports[alias] = port
	return s.write(ports)
}

func (s *PortStore) load() (map[string]int, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored portsDocument
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPorts, err)
	}
	if stored.SchemaVersion != portsSchemaVersion {
		return nil, ErrInvalidPorts
	}
	ports := make(map[string]int, len(stored.Ports))
	for alias, port := range stored.Ports {
		if validate.Alias(alias) != nil || !validPort(port) {
			return nil, ErrInvalidPorts
		}
		ports[alias] = port
	}
	return ports, nil
}

func (s *PortStore) write(ports map[string]int) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(portsDocument{SchemaVersion: portsSchemaVersion, Ports: ports}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.path(), portsTemporaryName, storage.FilePermission, contents)
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}
