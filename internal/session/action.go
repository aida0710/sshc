package session

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"time"
)

const (
	// ActionTokenTTL は確認トークンの有効期間である。確認ダイアログから即時に
	// 使用する一時トークンなので、有効期間を短くする。
	ActionTokenTTL = 2 * time.Minute
	// MaxActionTokensPerSession は、ひとつのセッションが固定できるメモリを制限する。
	MaxActionTokensPerSession = 32
)

// アクションの種別。ある種別に発行されたトークンは、他のどの種別にも使えない。
const (
	ActionReachability      = "diagnostics.reachability"
	ActionAuthentication    = "diagnostics.authentication"
	ActionKnownHostsDelete  = "known_hosts.delete"
	ActionKnownHostsScan    = "known_hosts.scan"
	ActionKnownHostsAdd     = "known_hosts.add"
	ActionRemoteKeyRegister = "remote_key.register"
	ActionRevealPrivateKey  = "private_key.reveal"
	ActionRevealCredential  = "credential.reveal"
	ActionPurgeTrashEntry   = "trash.purge"
	ActionSFTPDelete        = "sftp.delete"
	ActionSFTPChmod         = "sftp.chmod"
	ActionSnippetExecute    = "snippet.execute"
	ActionTerminalCommand   = "terminal.command.broadcast"
	ActionSyncForcePush     = "sync.force_push"
)

var (
	ErrInvalidAction  = errors.New("action token is not valid for this operation")
	ErrActionExpired  = errors.New("action token has expired")
	ErrUnknownSession = errors.New("session does not exist")
	ErrTooManyActions = errors.New("too many pending confirmations for this session")
)

var knownActionKinds = map[string]bool{
	ActionReachability:      true,
	ActionAuthentication:    true,
	ActionKnownHostsDelete:  true,
	ActionKnownHostsScan:    true,
	ActionKnownHostsAdd:     true,
	ActionRemoteKeyRegister: true,
	ActionRevealPrivateKey:  true,
	ActionRevealCredential:  true,
	ActionPurgeTrashEntry:   true,
	ActionSFTPDelete:        true,
	ActionSFTPChmod:         true,
	ActionSnippetExecute:    true,
	ActionTerminalCommand:   true,
	ActionSyncForcePush:     true,
}

// KnownActionKind は、kind がこのアプリケーションのいずれかの確認対象となる操作か
// を報告する。
func KnownActionKind(kind string) bool { return knownActionKinds[kind] }

// ActionRequest は、確認済みの操作をちょうどひとつ特定する。
//
// Evidence は確認ダイアログが表示していた内容のダイジェスト。通常は実行される
// ディレクティブか、編集対象ファイルの現在の内容である。そのため確認と実行の
// あいだに変化があれば、暗黙に別のものに適用されるのではなく、トークンが無効に
// なる。
type ActionRequest struct {
	Kind     string
	Target   string
	Evidence string
}

type actionRecord struct {
	tokenHash [sha256.Size]byte
	kind      string
	target    string
	evidence  string
	expiresAt time.Time
}

func (m *Manager) clock() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// IssueAction は確認をひとつ保存し、そのトークンを返す。トークンが返るのは一度
// きりで、保持されるのはそのハッシュだけ。セッションの秘密とまったく同じである。
//
// 表が満杯のときは場所を空けるのではなく拒否する。未使用のレコードを追い出せる
// なら、トークンを要求できる者は誰でも、ユーザーがすでに与えた確認を流し去り、
// 自分の選んだものに置き換えられてしまう。
func (m *Manager) IssueAction(sessionID string, request ActionRequest) (string, error) {
	if !KnownActionKind(request.Kind) || request.Target == "" {
		return "", ErrInvalidAction
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionValue, ok := m.sessions[sha256.Sum256([]byte(sessionID))]
	if !ok {
		return "", ErrUnknownSession
	}
	now := m.clock()
	expireLocked(sessionValue, now)
	if len(sessionValue.actions) >= MaxActionTokensPerSession {
		return "", ErrTooManyActions
	}

	value, err := token(m.random)
	if err != nil {
		return "", err
	}
	valueHash := sha256.Sum256([]byte(value))
	sessionValue.actions[valueHash] = actionRecord{
		tokenHash: valueHash,
		kind:      request.Kind,
		target:    request.Target,
		evidence:  request.Evidence,
		expiresAt: now.Add(ActionTokenTTL),
	}
	return value, nil
}

// ConsumeAction は確認をひとつ検証し、焼き捨てる。
//
// トークンは検査される前に取り除かれる。したがって、一致しない提示を別の操作に
// 対して再試行することはできない。どのエラーもトークンには言及せず、また、他の
// セッションに属するトークンは、単にこのセッションの表に存在しないというだけの
// ことである。
func (m *Manager) ConsumeAction(sessionID, presented string, request ActionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionValue, ok := m.sessions[sha256.Sum256([]byte(sessionID))]
	if !ok {
		return ErrUnknownSession
	}

	// 提示されたトークンの照会は、期限切れレコードの掃除より前に行う。そのため、
	// 時間切れになった確認は「期限切れ」として報告され、そもそも発行されなかった
	// ものと区別がつかない、という事態にはならない。
	presentedHash := sha256.Sum256([]byte(presented))
	record, found := sessionValue.actions[presentedHash]
	if !found {
		return ErrInvalidAction
	}
	delete(sessionValue.actions, presentedHash)

	now := m.clock()
	expireLocked(sessionValue, now)
	if now.After(record.expiresAt) {
		return ErrActionExpired
	}

	// 秘密そのものは、保存されたハッシュと定数時間で比較する。Bootstrap や
	// VerifyCSRF が使うのと同じ形なので、検証はマップがキーをどう比較するかには
	// 依存しない。kind、target、evidence のすべてが、いま求められている操作と一致
	// しなければならない。
	matched := subtle.ConstantTimeCompare(presentedHash[:], record.tokenHash[:]) &
		subtle.ConstantTimeCompare([]byte(record.kind), []byte(request.Kind)) &
		subtle.ConstantTimeCompare([]byte(record.target), []byte(request.Target)) &
		subtle.ConstantTimeCompare([]byte(record.evidence), []byte(request.Evidence))
	if matched != 1 {
		return ErrInvalidAction
	}
	return nil
}

func expireLocked(sessionValue Session, now time.Time) {
	for hash, record := range sessionValue.actions {
		if now.After(record.expiresAt) {
			delete(sessionValue.actions, hash)
		}
	}
}
