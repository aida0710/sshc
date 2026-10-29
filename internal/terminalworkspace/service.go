package terminalworkspace

import (
	"crypto/rand"
	"io"
	"slices"
	"sync"

	"sshc/internal/randomid"
	"sshc/internal/storage"
)

// Change は、ワークスペースの名前と表示するかの変更である。nil の項目は変えない。
type Change struct {
	Name   *string
	Hidden *bool
}

// Service は、ひとつのエンジンの中でワークスペースの定義の読み書きを直列化する。
type Service struct {
	store  store
	random io.Reader
	mutex  sync.Mutex
}

func NewService(workspace *storage.Workspace, random io.Reader) *Service {
	if random == nil {
		random = rand.Reader
	}
	return &Service{store: store{workspace: workspace}, random: random}
}

// List は、画面に並べる順に返す。
func (s *Service) List() ([]Workspace, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.store.load()
}

// Exists は、id のワークスペースがあるかを返す。
func (s *Service) Exists(id string) (bool, error) {
	workspaces, err := s.List()
	if err != nil {
		return false, err
	}
	return indexOf(workspaces, id) >= 0, nil
}

func indexOf(workspaces []Workspace, id string) int {
	return slices.IndexFunc(workspaces, func(w Workspace) bool { return w.ID == id })
}

// Create は、ワークスペースをひとつ作る。
func (s *Service) Create(name string, hidden bool) (Workspace, error) {
	name, err := normalizeName(name, false)
	if err != nil {
		return Workspace{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	workspaces, err := s.store.load()
	if err != nil {
		return Workspace{}, err
	}
	if len(workspaces) >= MaxWorkspaces {
		return Workspace{}, ErrLimit
	}
	id, err := randomid.UnusedID(s.random, func(candidate string) (bool, error) {
		return isBuiltIn(candidate) || indexOf(workspaces, candidate) >= 0, nil
	})
	if err != nil {
		return Workspace{}, err
	}
	created := Workspace{ID: id, Name: name, Hidden: hidden}
	if err := s.store.write(withCreated(workspaces, created)); err != nil {
		return Workspace{}, err
	}
	return created, nil
}

// withCreated は、作ったワークスペースを末尾に足す。末尾が agents のときは、その前に
// 入れる。普段は隠す agents を、利用者が動かさない限り一番下に保つためである。
func withCreated(workspaces []Workspace, created Workspace) []Workspace {
	at := len(workspaces)
	if at > 0 && workspaces[at-1].ID == AgentsID {
		at--
	}
	return slices.Insert(workspaces, at, created)
}

// Update は、名前と表示するかを変える。
func (s *Service) Update(id string, change Change) (Workspace, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	workspaces, err := s.store.load()
	if err != nil {
		return Workspace{}, err
	}
	index := indexOf(workspaces, id)
	if index < 0 {
		return Workspace{}, ErrNotFound
	}
	updated := workspaces[index]
	if change.Name != nil {
		if updated.Name, err = normalizeName(*change.Name, updated.BuiltIn()); err != nil {
			return Workspace{}, err
		}
	}
	if change.Hidden != nil {
		updated.Hidden = *change.Hidden
	}
	workspaces[index] = updated
	if err := s.store.write(workspaces); err != nil {
		return Workspace{}, err
	}
	return updated, nil
}

// Delete は、ワークスペースを消す。中のセッションは閉じずに、moveSessions で既定の
// ワークスペースへ移す。定義を書き換えてから移すので、移したあとに消えたワークスペースを
// 指すセッションは残らない。
func (s *Service) Delete(id string, moveSessions func(from, to string)) error {
	if isBuiltIn(id) {
		return ErrBuiltIn
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	workspaces, err := s.store.load()
	if err != nil {
		return err
	}
	index := indexOf(workspaces, id)
	if index < 0 {
		return ErrNotFound
	}
	if err := s.store.write(slices.Delete(workspaces, index, index+1)); err != nil {
		return err
	}
	moveSessions(id, DefaultID)
	return nil
}

// Move は、id のワークスペースを beforeID のワークスペースの前へ移す。beforeID が空なら
// 末尾へ移す。ほかのワークスペースの並びは変えないので、別の画面で作られたばかりの
// ワークスペースを知らない画面からの移動でも、そのワークスペースの場所は崩れない。
func (s *Service) Move(id, beforeID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	workspaces, err := s.store.load()
	if err != nil {
		return err
	}
	index := indexOf(workspaces, id)
	if index < 0 {
		return ErrNotFound
	}
	if beforeID == id {
		return nil
	}
	moved := workspaces[index]
	rest := slices.Delete(workspaces, index, index+1)
	at := len(rest)
	if beforeID != "" {
		if at = indexOf(rest, beforeID); at < 0 {
			return ErrNotFound
		}
	}
	return s.store.write(slices.Insert(rest, at, moved))
}
