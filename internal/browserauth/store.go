// Package browserauth は、この端末で一度登録したブラウザを記録する。
//
// ブラウザへ渡した資格情報そのものは保存せず、ワークスペースにはハッシュだけを
// 残す。この状態は端末固有であり、リモート同期の対象にはしない。
//
// 登録は固定 loopback origin に置く再利用可能な bearer なので、奪われた場合の
// 被害を時間と回数で区切る。recover のたびに token を差し替え（rotation）、
// 差し替え前の token が猶予の後に提示されたら盗まれたものとみなして登録ごと
// 失効し、使われない登録は期限で消す。port が変わればすべて失効する。
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
	"time"

	"sshc/internal/storage"
)

const (
	SchemaVersion    = 2
	PathRelative     = "sshc/browser-registrations.json"
	MaxRegistrations = 16
	temporaryName    = ".browser-registrations-"

	// RegistrationLifetime は、最後に使われてから登録が生きる期間。ブックマークから
	// の復旧は日常的なので長めに取り、使わなくなったブラウザの登録だけを消す。
	RegistrationLifetime = 30 * 24 * time.Hour
	// rotationGrace は、差し替え前の token をまだ受け付ける時間。engine 起動直後に
	// 同じブラウザの複数 tab が同時に recover すると、後の tab は差し替え前の token を
	// 持ったまま来る。その tab にも同じ新 token を渡し、猶予を過ぎた提示だけを
	// 再利用（盗難）として扱う。
	rotationGrace = time.Minute
)

var ErrInvalidDocument = errors.New("browser registrations document is invalid")

// registration は、登録済みブラウザ 1 つの状態である。token は保存せずハッシュだけを持つ。
type registration struct {
	Hash string `json:"hash"`
	// Previous は、直前の rotation で退役した token のハッシュ。猶予の後に提示されたら
	// この登録は盗まれている。
	Previous   string    `json:"previous,omitempty"`
	IssuedAt   time.Time `json:"issuedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

type document struct {
	SchemaVersion int            `json:"schemaVersion"`
	Port          int            `json:"port,omitempty"`
	Registrations []registration `json:"registrations"`
}

// rotation は、退役した token を提示した同じブラウザの別 tab に、猶予のあいだだけ
// 同じ新 token を渡すためにメモリに置く。ディスクには書かない。
type rotation struct {
	token string
	until time.Time
}

// Store は、登録情報の検証と更新をひとつのengine内で直列化する。
type Store struct {
	workspace *storage.Workspace
	random    io.Reader
	now       func() time.Time
	mutex     sync.Mutex
	rotations map[string]rotation
}

func NewStore(workspace *storage.Workspace, random io.Reader) *Store {
	return &Store{workspace: workspace, random: random, now: time.Now, rotations: map[string]rotation{}}
}

// WithClock は、期限と猶予の判定に使う時計を差し替える。
func (s *Store) WithClock(now func() time.Time) *Store {
	s.now = now
	return s
}

func (s *Store) Path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

// HasRegistrations は、この端末に期限内の登録済みブラウザがあるかを返す。
func (s *Store) HasRegistrations() (bool, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return false, err
	}
	return len(s.live(stored.Registrations)) > 0, nil
}

// Port returns the device-local browser origin port, if one has been selected.
func (s *Store) Port() (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	return stored.Port, err
}

// SetPort persists the browser origin without putting it in sync metadata.
// A registration is an origin capability. Moving to another port changes that
// origin, so every capability issued for the previous port is revoked in the
// same atomic state update. Reusing the same port preserves restart recovery.
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
	stored.Registrations = []registration{}
	s.rotations = map[string]rotation{}
	return s.write(stored)
}

// Recover は、ブラウザが提示した登録 token を検証し、受け付けたなら差し替え後の
// 新しい token を返す。呼び出し側は新しい token を保存させる。
//
// 差し替え前の token は猶予のあいだだけ同じ新 token を返す。猶予の後に提示された
// 退役 token は盗まれたものとみなし、その登録を消して拒否する。
func (s *Store) Recover(presented string) (string, bool, error) {
	if !validToken(presented) {
		return "", false, nil
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return "", false, err
	}
	now := s.now()
	presentedHash := hashToken(presented)
	live := s.live(stored.Registrations)
	if index := indexOf(live, presentedHash, currentHash); index >= 0 {
		token, err := mint(s.random)
		if err != nil {
			return "", false, err
		}
		live[index].Previous = live[index].Hash
		live[index].Hash = hashToken(token)
		live[index].LastUsedAt = now
		s.rotations[presentedHash] = rotation{token: token, until: now.Add(rotationGrace)}
		stored.Registrations = live
		if err := s.write(stored); err != nil {
			return "", false, err
		}
		return token, true, nil
	}
	if recent, ok := s.rotations[presentedHash]; ok && now.Before(recent.until) {
		return recent.token, true, nil
	}
	if index := indexOf(live, presentedHash, previousHash); index >= 0 {
		// 猶予を過ぎた退役 token の提示。正規のブラウザは既に新しい token で
		// 動いているので、これは複製された token である。登録ごと失効する。
		stored.Registrations = append(live[:index:index], live[index+1:]...)
		if err := s.write(stored); err != nil {
			return "", false, err
		}
	}
	return "", false, nil
}

// Forget は、提示された token の登録を消す。サインアウトしたブラウザは engine の
// 再起動後にこの token で入り直せなくなる。差し替え猶予中の旧 token も同じ登録を
// 指すので受け付ける。知らない token は何もせず false を返す。
func (s *Store) Forget(presented string) (bool, error) {
	if !validToken(presented) {
		return false, nil
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	stored, err := s.load()
	if err != nil {
		return false, err
	}
	presentedHash := hashToken(presented)
	live := s.live(stored.Registrations)
	index := indexOf(live, presentedHash, currentHash)
	if index < 0 {
		index = indexOf(live, presentedHash, previousHash)
	}
	if index < 0 {
		return false, nil
	}
	delete(s.rotations, live[index].Previous)
	delete(s.rotations, presentedHash)
	stored.Registrations = append(live[:index:index], live[index+1:]...)
	if err := s.write(stored); err != nil {
		return false, err
	}
	return true, nil
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
	live := s.live(stored.Registrations)
	if validToken(presented) && indexOf(live, hashToken(presented), currentHash) >= 0 {
		return "", false, nil
	}
	token, err := mint(s.random)
	if err != nil {
		return "", false, err
	}
	now := s.now()
	if len(live) == MaxRegistrations {
		live = append(live[:0], live[1:]...)
	}
	stored.Registrations = append(live, registration{Hash: hashToken(token), IssuedAt: now, LastUsedAt: now})
	if err := s.write(stored); err != nil {
		return "", false, err
	}
	return token, true, nil
}

// live は、期限内の登録だけを返す。
func (s *Store) live(registrations []registration) []registration {
	now := s.now()
	kept := make([]registration, 0, len(registrations))
	for _, entry := range registrations {
		if now.Before(entry.LastUsedAt.Add(RegistrationLifetime)) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func currentHash(entry registration) string  { return entry.Hash }
func previousHash(entry registration) string { return entry.Previous }

// indexOf は、field が hash と一致する登録の位置を定数時間の比較で探す。
func indexOf(registrations []registration, hash string, field func(registration) string) int {
	found := -1
	for index, entry := range registrations {
		candidate := field(entry)
		if candidate != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(hash)) == 1 {
			found = index
		}
	}
	return found
}

func mint(random io.Reader) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func validToken(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *Store) load() (document, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return document{SchemaVersion: SchemaVersion, Registrations: []registration{}}, nil
	}
	if err != nil {
		return document{}, err
	}
	var header struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if json.Unmarshal(contents, &header) == nil && header.SchemaVersion > 0 && header.SchemaVersion < SchemaVersion {
		// 古い形式は読まない。登録はブラウザを一度開き直せば作り直せるので、
		// 形式が変わった端末では登録なしから始める。
		return document{SchemaVersion: SchemaVersion, Registrations: []registration{}}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var stored document
	if err := decoder.Decode(&stored); err != nil {
		return document{}, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF || stored.SchemaVersion != SchemaVersion ||
		len(stored.Registrations) > MaxRegistrations ||
		(stored.Port != 0 && (stored.Port < 1024 || stored.Port > 65535)) {
		return document{}, ErrInvalidDocument
	}
	seen := make(map[string]bool, len(stored.Registrations))
	for _, entry := range stored.Registrations {
		if !validHash(entry.Hash) || (entry.Previous != "" && !validHash(entry.Previous)) ||
			seen[entry.Hash] || entry.IssuedAt.IsZero() || entry.LastUsedAt.IsZero() {
			return document{}, ErrInvalidDocument
		}
		seen[entry.Hash] = true
	}
	stored.Registrations = append([]registration(nil), stored.Registrations...)
	return stored, nil
}

func validHash(encoded string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(decoded) == sha256.Size
}

func (s *Store) write(stored document) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	stored.SchemaVersion = SchemaVersion
	if stored.Registrations == nil {
		stored.Registrations = []registration{}
	}
	contents, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return storage.WriteAtomicFile(s.workspace.FileSystem(), s.Path(), temporaryName, storage.FilePermission, contents)
}
