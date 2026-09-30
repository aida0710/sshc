package session

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"sync"
	"time"

	"sshc/internal/randomid"
)

var (
	ErrInvalidBootstrap = errors.New("invalid bootstrap token")
	ErrBootstrapUsed    = errors.New("bootstrap token already used")
	ErrInvalidLifetime  = errors.New("session lifetime must be positive")
)

type Credentials struct {
	SessionID string
	CSRFToken string
}

type Session struct {
	// csrfTokens keeps a small, bounded set so tabs which share one session do
	// not revoke each other when one refreshes its token. Only hashes are kept;
	// a cookie without one of the origin-scoped tokens is still useless.
	csrfTokens issuedCSRFTokens
	// actions は、このセッションの未使用の確認を保持する。キーは配られたトークンの
	// ダイジェストで、セッション自身のキーの付け方とまったく同じである。このマップは
	// Session 値のすべてのコピーで共有されており、それによってアクション用のヘルパー
	// は、もう一度検索することなくここへ到達できる。
	actions map[[sha256.Size]byte]actionRecord
	// expiresAt は絶対期限。使い続けていても、これを過ぎると切れる。
	expiresAt time.Time
	// idleTimeout のあいだ使われなければ、絶対期限の前でも切れる。ゼロなら
	// 絶対期限だけで切れる。
	idleTimeout time.Duration
	// lastUsedAt は最後に使われた時刻。無操作の期間はここから数える。
	lastUsedAt time.Time
	// inFlight は処理中のリクエストの数。処理中は無操作の期限で切らない。
	// 1 本の長いダウンロードのあいだに期限が来ると、続くリクエストが断られる。
	inFlight int
	// registration は、このセッションで入り直したブラウザ登録の識別子。登録が
	// 消えたら、RevokeRegistration でこのセッションも失効させる。
	registration string
}

// Expiry は、発行するセッションがいつ切れるかを決める。
type Expiry struct {
	// Lifetime は発行からの絶対期限。
	Lifetime time.Duration
	// IdleTimeout は無操作の期限。ゼロなら絶対期限だけで切れる。
	IdleTimeout time.Duration
}

// ブラウザのセッションは engine の再起動で消えるが、動き続ける engine では
// cookie が盗まれた場合の有効期間を区切る。切れたセッションはブラウザ登録から
// 黙って入り直せるので、利用者の手間は増えない。
const (
	// 一晩置いた翌朝は入り直し、席を離れた程度では切らない。
	BrowserSessionIdleTimeout = 12 * time.Hour
	// 使い続けていても 1 週間で必ず入り直す。
	BrowserSessionLifetime = 7 * 24 * time.Hour
	// lastUsedAt の更新は書き込みロックなので、この間隔より短くは更新しない。
	lastUsedCoalesce = time.Minute
)

var browserExpiry = Expiry{Lifetime: BrowserSessionLifetime, IdleTimeout: BrowserSessionIdleTimeout}

type Manager struct {
	mu            sync.RWMutex
	random        io.Reader
	bootstrapHash [sha256.Size]byte
	bootstrapUsed bool
	sessions      map[[sha256.Size]byte]Session

	// Now は、セッションとアクショントークンの失効に使う時計。本番では nil で
	// time.Now が使われる。テストは、マネージャが共有される前に一度だけこれを設定する。
	Now func() time.Time
}

func NewManager(random io.Reader) (*Manager, string, error) {
	bootstrap, err := randomid.Token(random)
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
	fresh, err := randomid.Token(m.random)
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bootstrapHash = sha256.Sum256([]byte(fresh))
	m.bootstrapUsed = false
	return fresh, nil
}

// BootstrapForSession はone-time bootstrapを消費し、すでに有効なcookieがあれば
// そのsessionへ新しいtab用CSRF tokenを追加する。cookieを差し替えないことで、
// 同じブラウザで開いている別tabを切断しない。
func (m *Manager) BootstrapForSession(presented, existingSessionID string) (Credentials, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.bootstrapUsed {
		return Credentials{}, false, ErrBootstrapUsed
	}

	presentedHash := sha256.Sum256([]byte(presented))
	if subtle.ConstantTimeCompare(presentedHash[:], m.bootstrapHash[:]) != 1 {
		return Credentials{}, false, ErrInvalidBootstrap
	}

	credentials, setCookie, err := m.joinOrIssueLocked(existingSessionID)
	if err != nil {
		return Credentials{}, false, err
	}

	m.bootstrapUsed = true
	return credentials, setCookie, nil
}

// JoinOrIssue adds a tab token to an existing browser session, or creates a session when
// the engine was restarted and the browser cookie belongs to the previous process.
// JoinOrIssue は、ブラウザ登録 registration で入り直したブラウザに、有効な既存の
// セッションがあればそこへ加え、無ければ新しく発行する。どちらの場合もセッションを
// その登録に結び付けるので、登録が消えたときに RevokeRegistration で失効できる。
func (m *Manager) JoinOrIssue(existingSessionID, registration string) (Credentials, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	credentials, setCookie, err := m.joinOrIssueLocked(existingSessionID)
	if err != nil {
		return Credentials{}, false, err
	}
	key := sha256.Sum256([]byte(credentials.SessionID))
	entered := m.sessions[key]
	entered.registration = registration
	m.sessions[key] = entered
	return credentials, setCookie, nil
}

func (m *Manager) joinOrIssueLocked(existingSessionID string) (Credentials, bool, error) {
	if existing, ok := m.sessionLocked(existingSessionID); ok {
		csrf, err := randomid.Token(m.random)
		if err != nil {
			return Credentials{}, false, err
		}
		key := sha256.Sum256([]byte(existingSessionID))
		existing.csrfTokens = existing.csrfTokens.add(csrf)
		m.sessions[key] = existing
		return Credentials{SessionID: existingSessionID, CSRFToken: csrf}, false, nil
	}
	credentials, err := m.issueBrowserLocked()
	return credentials, true, err
}

// RevokeRegistration は、ブラウザ登録 registration で入り直したセッションを
// すべて失効させ、その数を返す。
func (m *Manager) RevokeRegistration(registration string) int {
	if registration == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	revoked := 0
	for key, sessionValue := range m.sessions {
		if sessionValue.registration == registration {
			delete(m.sessions, key)
			revoked++
		}
	}
	return revoked
}

// issueBrowserLocked はブラウザ用のセッションを、絶対期限とアイドル期限付きで発行する。
func (m *Manager) issueBrowserLocked() (Credentials, error) {
	now := m.clock()
	m.pruneExpiredLocked(now)
	return m.issueLocked(now, browserExpiry)
}

// IssueExpiring は、ブラウザ用 bootstrap を消費せず、expiry に従って切れる
// セッションを発行する。CLI の一回の実行にだけ通常 API の権限を貸す用途である。
func (m *Manager) IssueExpiring(expiry Expiry) (Credentials, error) {
	if expiry.Lifetime <= 0 || expiry.IdleTimeout < 0 {
		return Credentials{}, ErrInvalidLifetime
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock()
	m.pruneExpiredLocked(now)
	return m.issueLocked(now, expiry)
}

func (m *Manager) pruneExpiredLocked(now time.Time) {
	for key, sessionValue := range m.sessions {
		if sessionValue.expired(now) {
			delete(m.sessions, key)
		}
	}
}

func (s Session) expired(now time.Time) bool {
	if !s.expiresAt.IsZero() && !now.Before(s.expiresAt) {
		return true
	}
	return s.idleTimeout > 0 && s.inFlight == 0 && now.Sub(s.lastUsedAt) >= s.idleTimeout
}

func (m *Manager) issueLocked(now time.Time, expiry Expiry) (Credentials, error) {
	sessionID, err := randomid.Token(m.random)
	if err != nil {
		return Credentials{}, err
	}
	csrf, err := randomid.Token(m.random)
	if err != nil {
		return Credentials{}, err
	}

	m.sessions[sha256.Sum256([]byte(sessionID))] = Session{
		csrfTokens:  newIssuedCSRFTokens(csrf),
		actions:     make(map[[sha256.Size]byte]actionRecord),
		expiresAt:   now.Add(expiry.Lifetime),
		idleTimeout: expiry.IdleTimeout,
		lastUsedAt:  now,
	}
	return Credentials{SessionID: sessionID, CSRFToken: csrf}, nil
}

// Revoke はセッションを直ちに削除し、実際に存在したときだけ true を返す。
func (m *Manager) Revoke(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := sha256.Sum256([]byte(sessionID))
	if _, ok := m.sessions[key]; !ok {
		return false
	}
	delete(m.sessions, key)
	return true
}

// sessionLocked は有効なセッションを返し、使った時刻を進める。呼び出し側は
// m.mu の書き込みロックを保持しなければならない。期限に達したセッションは
// 検索時に削除する。
func (m *Manager) sessionLocked(sessionID string) (Session, bool) {
	key := sha256.Sum256([]byte(sessionID))
	sessionValue, ok := m.sessions[key]
	if !ok {
		return Session{}, false
	}
	now := m.clock()
	if sessionValue.expired(now) {
		delete(m.sessions, key)
		return Session{}, false
	}
	if now.Sub(sessionValue.lastUsedAt) >= lastUsedCoalesce {
		sessionValue.lastUsedAt = now
		m.sessions[key] = sessionValue
	}
	return sessionValue, true
}

// RenewCSRF は、有効な現在のtokenを持つpageへ新しいCSRF tokenを発行する。
//
// Cookieはportへ束縛されないので、提示token自体もここで再検証する。複数tabが同じ
// sessionを使う場合に一方の更新で他方を切断しないよう、提示tokenは退役させず、
// 置き換えられたtokenとして上限付きで保持する。更新したページはもう使わないので、
// 上限を越えたときは使われているtokenより先に退役する。
func (m *Manager) RenewCSRF(sessionID, presented string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := sha256.Sum256([]byte(sessionID))
	existing, ok := m.sessionLocked(sessionID)
	if !ok {
		return "", false
	}
	index := existing.csrfTokens.find(presented)
	if index < 0 {
		return "", false
	}
	csrf, err := randomid.Token(m.random)
	if err != nil {
		return "", false
	}
	// 提示tokenを最後尾へ移してから足すので、上限に達していても提示tokenは
	// 押し出されず、新しいtokenの1つ前に残る。
	tokens := existing.csrfTokens.markUsed(index).add(csrf)
	existing.csrfTokens = tokens.markReplaced(len(tokens) - 2)
	m.sessions[key] = existing
	return csrf, true
}

// BeginRequest は、そのセッションで 1 本のリクエストを処理し始める。セッションが
// 有効なら、処理を終えたときに呼ぶ endRequest を返す。処理中のセッションは無操作の
// 期限で切れず、無操作の期間は処理を終えた時刻から数え直す。
//
// Session 内の actions マップは Manager のロックで保護する必要があるため、呼び出し側へ
// Session 自体は返さない。
func (m *Manager) BeginRequest(sessionID string) (endRequest func(), ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionValue, ok := m.sessionLocked(sessionID)
	if !ok {
		return nil, false
	}
	key := sha256.Sum256([]byte(sessionID))
	sessionValue.inFlight++
	m.sessions[key] = sessionValue
	return func() { m.endRequest(key) }, true
}

func (m *Manager) endRequest(key [sha256.Size]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionValue, ok := m.sessions[key]
	if !ok {
		// 処理中に失効させた。
		return
	}
	sessionValue.inFlight--
	sessionValue.lastUsedAt = m.clock()
	m.sessions[key] = sessionValue
}

func (m *Manager) VerifyCSRF(sessionID, csrf string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionValue, ok := m.sessionLocked(sessionID)
	if !ok {
		return false
	}
	index := sessionValue.csrfTokens.find(csrf)
	if index < 0 {
		return false
	}
	sessionValue.csrfTokens = sessionValue.csrfTokens.markUsed(index)
	m.sessions[sha256.Sum256([]byte(sessionID))] = sessionValue
	return true
}
