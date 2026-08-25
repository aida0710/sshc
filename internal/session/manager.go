package session

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"
)

var (
	ErrInvalidBootstrap = errors.New("invalid bootstrap token")
	ErrBootstrapUsed    = errors.New("bootstrap token already used")
)

type Credentials struct {
	SessionID string
	CSRFToken string
}

type Session struct {
	// csrfHashes keeps a small, bounded set so tabs which share one session do
	// not revoke each other when one refreshes its token. Only hashes are kept;
	// a cookie without one of the origin-scoped tokens is still useless.
	csrfHashes [][sha256.Size]byte
	// actions は、このセッションの未使用の確認を保持する。キーは配られたトークンの
	// ダイジェストで、セッション自身のキーの付け方とまったく同じである。このマップは
	// Session 値のすべてのコピーで共有されており、それによってアクション用のヘルパー
	// は、もう一度検索することなくここへ到達できる。
	actions map[[sha256.Size]byte]actionRecord
}

// MaxCSRFTokensPerSession bounds memory and the lifetime of an abandoned tab
// token. A ninth renewal evicts the oldest token.
const MaxCSRFTokensPerSession = 8

type Manager struct {
	mu            sync.RWMutex
	random        io.Reader
	bootstrapHash [sha256.Size]byte
	bootstrapUsed bool
	sessions      map[[sha256.Size]byte]Session

	// Now は、アクショントークンの失効に使う時計。本番では nil で time.Now が使われる。
	// テストは、マネージャが共有される前に一度だけこれを設定する。
	Now func() time.Time
}

func token(random io.Reader) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func NewManager(random io.Reader) (*Manager, string, error) {
	bootstrap, err := token(random)
	if err != nil {
		return nil, "", err
	}

	return &Manager{
		random:        random,
		bootstrapHash: sha256.Sum256([]byte(bootstrap)),
		sessions:      make(map[[sha256.Size]byte]Session),
	}, bootstrap, nil
}

// Reissue は新しいブートストラップトークンを発行し、マネージャが持つものを置き換える。
//
// ブートストラップは初回の使用で消費される。これができるまでは、新しいプロセスだけ
// が次の一つを表示していた。ユーザーがアプリケーションを起動して URL が表示される
// なら問題ないが、標準出力がどこにも届かないバックグラウンドエージェントとして動く
// 場合は役に立たない。再発行を求めるのはコマンドラインであり、そもそも求めるには、
// このユーザーしか読めないファイルを読む必要がある。
//
// すでに確立しているセッションは確立したままである。これが置き換えるのは、まだ
// セッションを持たないブラウザのためのアクセス URLだけだ。
func (m *Manager) Reissue() (string, error) {
	fresh, err := token(m.random)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bootstrapHash = sha256.Sum256([]byte(fresh))
	m.bootstrapUsed = false
	return fresh, nil
}

func (m *Manager) Bootstrap(presented string) (Credentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.bootstrapUsed {
		return Credentials{}, ErrBootstrapUsed
	}

	presentedHash := sha256.Sum256([]byte(presented))
	if subtle.ConstantTimeCompare(presentedHash[:], m.bootstrapHash[:]) != 1 {
		return Credentials{}, ErrInvalidBootstrap
	}

	sessionID, err := token(m.random)
	if err != nil {
		return Credentials{}, err
	}
	csrf, err := token(m.random)
	if err != nil {
		return Credentials{}, err
	}

	m.bootstrapUsed = true
	m.sessions[sha256.Sum256([]byte(sessionID))] = Session{
		csrfHashes: [][sha256.Size]byte{sha256.Sum256([]byte(csrf))},
		actions:    make(map[[sha256.Size]byte]actionRecord),
	}
	return Credentials{SessionID: sessionID, CSRFToken: csrf}, nil
}

// RenewCSRF は、有効な現在のtokenを持つpageへ新しいCSRF tokenを発行する。
//
// Cookieはportへ束縛されないので、提示token自体もここで再検証する。複数tabが同じ
// sessionを使う場合に一方の更新で他方を切断しないよう、既存tokenは上限付きで保持する。
// 上限を越えた最古tokenは退役する。
func (m *Manager) RenewCSRF(sessionID, presented string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := sha256.Sum256([]byte(sessionID))
	existing, ok := m.sessions[key]
	if !ok {
		return "", false
	}
	if !csrfMatches(existing.csrfHashes, presented) {
		return "", false
	}
	csrf, err := token(m.random)
	if err != nil {
		return "", false
	}
	freshHash := sha256.Sum256([]byte(csrf))
	if len(existing.csrfHashes) < MaxCSRFTokensPerSession {
		existing.csrfHashes = append(existing.csrfHashes, freshHash)
	} else {
		copy(existing.csrfHashes, existing.csrfHashes[1:])
		existing.csrfHashes[len(existing.csrfHashes)-1] = freshHash
	}
	m.sessions[key] = existing
	return csrf, true
}

// Authenticate は、セッションが存在するかどうかだけを報告する。
//
// Session 内の actions マップは Manager のロックで保護する必要があるため、呼び出し側へ
// Session 自体は公開しない。
func (m *Manager) Authenticate(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.sessions[sha256.Sum256([]byte(sessionID))]
	return ok
}

func (m *Manager) VerifyCSRF(sessionID, csrf string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionValue, ok := m.sessions[sha256.Sum256([]byte(sessionID))]
	if !ok {
		return false
	}
	return csrfMatches(sessionValue.csrfHashes, csrf)
}

func csrfMatches(hashes [][sha256.Size]byte, presented string) bool {
	presentedHash := sha256.Sum256([]byte(presented))
	matched := 0
	for _, candidate := range hashes {
		matched |= subtle.ConstantTimeCompare(presentedHash[:], candidate[:])
	}
	return matched == 1
}
