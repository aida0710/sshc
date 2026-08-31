// Package browserauth は、この端末で一度登録したブラウザを記録する。
//
// ブラウザへ渡した資格情報そのものは保存せず、ワークスペースにはハッシュだけを
// 残す。この状態は端末固有であり、リモート同期の対象にはしない。
package browserauth

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sync"

	"sshc/internal/storage"
)

const (
	SchemaVersion    = 1
	PathRelative     = "sshc/browser-registrations.json"
	MaxRegistrations = 16
	temporaryName    = ".browser-registrations-"
)

var ErrInvalidDocument = errors.New("browser registrations document is invalid")

type document struct {
	SchemaVersion int      `json:"schemaVersion"`
	Port          int      `json:"port,omitempty"`
	Hashes        []string `json:"hashes"`
}

// Store は、登録情報の検証と更新をひとつのengine内で直列化する。
type Store struct {
	workspace *storage.Workspace
	random    io.Reader
	mutex     sync.Mutex
}

func NewStore(workspace *storage.Workspace, random io.Reader) *Store {
	return &Store{workspace: workspace, random: random}
}

func (s *Store) Path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// HasRegistrations は、この端末に登録済みブラウザがあるかを返す。
func (s *Store) HasRegistrations() (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	return len(stored.Hashes) > 0, err
}

// Port returns the device-local browser origin port, if one has been selected.
func (s *Store) Port() (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	return stored.Port, err
}

// SetPort persists the desktop browser origin without putting it in sync metadata.
func (s *Store) SetPort(port int) error {
	if port < 1024 || port > 65535 {
		return ErrInvalidDocument
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return err
	}
	if stored.Port == port {
		return nil
	}
	stored.Port = port
	return s.write(stored)
}

// Verify はブラウザが提示した資格情報を、保存済みハッシュすべてに対して
// 定数時間比較する。登録数には小さな上限がある。
func (s *Store) Verify(presented string) bool {
	if !validToken(presented) {
		return false
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return false
	}
	presentedHash := sha256.Sum256([]byte(presented))
	matched := 0
	for _, encoded := range stored.Hashes {
		candidate, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return false
		}
		matched |= subtle.ConstantTimeCompare(presentedHash[:], candidate)
	}
	return matched == 1
}

// Register returns an existing valid registration unchanged. A one-time bootstrap may
// call it with an empty or stale value to enrol the current browser and receive a new token.
func (s *Store) Register(presented string) (string, bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return "", false, err
	}
	if validToken(presented) && matches(stored.Hashes, presented) {
		return "", false, nil
	}
	token, err := mint(s.random)
	if err != nil {
		return "", false, err
	}
	digest := sha256.Sum256([]byte(token))
	encoded := base64.RawURLEncoding.EncodeToString(digest[:])
	if len(stored.Hashes) == MaxRegistrations {
		stored.Hashes = append(stored.Hashes[:0], stored.Hashes[1:]...)
	}
	stored.Hashes = append(stored.Hashes, encoded)
	if err := s.write(stored); err != nil {
		return "", false, err
	}
	return token, true, nil
}

func mint(random io.Reader) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validToken(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func matches(hashes []string, presented string) bool {
	presentedHash := sha256.Sum256([]byte(presented))
	matched := 0
	for _, encoded := range hashes {
		candidate, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return false
		}
		matched |= subtle.ConstantTimeCompare(presentedHash[:], candidate)
	}
	return matched == 1
}

func (s *Store) load() (document, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return document{SchemaVersion: SchemaVersion, Hashes: []string{}}, nil
	}
	if err != nil {
		return document{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var stored document
	if err := decoder.Decode(&stored); err != nil {
		return document{}, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF || stored.SchemaVersion != SchemaVersion || len(stored.Hashes) > MaxRegistrations ||
		(stored.Port != 0 && (stored.Port < 1024 || stored.Port > 65535)) {
		return document{}, ErrInvalidDocument
	}
	seen := make(map[string]bool, len(stored.Hashes))
	for _, encoded := range stored.Hashes {
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(decoded) != sha256.Size || seen[encoded] {
			return document{}, ErrInvalidDocument
		}
		seen[encoded] = true
	}
	stored.Hashes = append([]string(nil), stored.Hashes...)
	return stored, nil
}

func (s *Store) write(stored document) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	stored.SchemaVersion = SchemaVersion
	contents, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.Path(), temporaryName, storage.FilePermission, contents)
}
