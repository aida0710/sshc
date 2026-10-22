package remoteengine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sshc/internal/handoff"
)

// handoffCommand は、接続先で、そのエンジンの handoff（待ち受けのポートと CLI の秘密）を
// 読む固定のコマンドである。接続先の sshc CLI が読むのと同じファイルを、同じ利用者として
// 読む。無ければ（エンジンが動いていなければ）0 以外で終わる。
const handoffCommand = `sh -c 'cat "$HOME/.ssh/sshc/cli"'`

// handoffLimit は、handoffCommand が終わるまでの上限である。小さいファイルを読むだけで、
// SSH の往復を含めても数秒で終わる。
const handoffLimit = 10 * time.Second

// Header names and the cookie the remote engine's own API expects. They are
// spelled here rather than imported because httpserver imports this package.
const (
	sessionCookieName = "sshc_session"
	csrfHeaderName    = "X-SSHC-CSRF"
	challengePath     = "/cli/challenge"
	cliSessionPath    = "/cli/session"
	cliStatusPath     = "/cli/status"
)

var (
	// ErrNotRunning は、接続先で sshc のエンジンが動いていないことを表す。
	ErrNotRunning = errors.New("the sshc engine on the remote host is not running")
	// ErrUnproven は、接続先のエンジンが handoff の秘密を持つことを示せなかったことを表す。
	ErrUnproven = errors.New("the remote engine could not prove that it owns its handoff")
)

// VersionMismatchError は、このマシンと接続先の sshc のバージョンが違うことを表す。画面は接続先の
// エンジンの API をそのまま使うので、同じバージョンでなければ取り次がない。
type VersionMismatchError struct{ Local, Remote string }

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("the remote engine runs sshc %s, this engine runs %s", e.Remote, e.Local)
}

// remoteSession は、接続先のエンジンの API を使うための資格である。CLI と同じく
// /cli/session で受け取る。
type remoteSession struct {
	port    int
	secret  string
	version string
	cookie  string
	csrf    string
}

func (s *remoteSession) origin() string { return "http://" + loopbackHost(s.port) }

// remoteEngine は、実行先に足した接続先のエンジンひとつである。
type remoteEngine struct {
	link         *hostLink
	localVersion string
	transport    *http.Transport

	mutex   sync.Mutex
	session *remoteSession
}

func newRemoteEngine(link *hostLink, localVersion string) *remoteEngine {
	transport := &http.Transport{
		// 宛先は接続先から見た 127.0.0.1:<ポート> である。
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			link, err := link.live(ctx)
			if err != nil {
				return nil, err
			}
			return link.DialContext(ctx, "tcp", address)
		},
		DisableCompression: true,
	}
	link.whenEnded(transport.CloseIdleConnections)
	return &remoteEngine{link: link, localVersion: localVersion, transport: transport}
}

func (e *remoteEngine) client() *http.Client {
	return &http.Client{
		Transport: e.transport,
		// 接続先のエンジンは redirect を返さない。返されても、秘密や cookie を付けたまま
		// 別の場所へ送らない。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// current は、使える資格を返す。まだ無ければ、接続先で handoff を読んで受け取る。
func (e *remoteEngine) current(ctx context.Context) (*remoteSession, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.session != nil {
		return e.session, nil
	}
	session, err := e.attach(ctx)
	if err != nil {
		return nil, err
	}
	e.session = session
	return session, nil
}

// forget は、断られた資格を捨てる。次の要求で受け取り直す。
func (e *remoteEngine) forget(stale *remoteSession) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.session == stale {
		e.session = nil
	}
}

// attach は、接続先の handoff を読み、相手がその秘密を持つエンジンかを確かめてから、
// API の資格を受け取る。手順はこのマシンの sshc CLI と同じである。
func (e *remoteEngine) attach(ctx context.Context) (*remoteSession, error) {
	link, err := e.link.live(ctx)
	if err != nil {
		return nil, err
	}
	result, err := link.Run(ctx, handoffCommand, handoffLimit)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, ErrNotRunning
	}
	document, err := handoff.Parse(result.Stdout)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	if document.Version != e.localVersion {
		return nil, &VersionMismatchError{Local: e.localVersion, Remote: document.Version}
	}
	port, err := strconv.Atoi(strings.TrimPrefix(document.URL, "http://"+handoff.EngineHost+":"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	session := &remoteSession{port: port, secret: document.Secret, version: document.Version}
	if err := e.prove(ctx, session); err != nil {
		return nil, err
	}
	if err := e.openSession(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// prove は乱数を送り、handoff の秘密で署名した答えが返ることを確かめる。handoff だけが
// 残り、同じポートを別のプロセスが取っていると、秘密をそのプロセスへ渡してしまう。
func (e *remoteEngine) prove(ctx context.Context, session *remoteSession) error {
	challenge, err := handoff.MintChallenge(rand.Reader)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, session.origin()+challengePath, nil)
	if err != nil {
		return err
	}
	request.Header.Set(handoff.ChallengeHeader, challenge)
	response, err := e.client().Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent ||
		!handoff.VerifyProof(session.secret, challenge, response.Header.Get(handoff.ProofHeader)) {
		return ErrUnproven
	}
	return nil
}

// openSession は、handoff の秘密と引き換えに、API の cookie と CSRF のトークンを受け取る。
func (e *remoteEngine) openSession(ctx context.Context, session *remoteSession) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, session.origin()+cliSessionPath, nil)
	if err != nil {
		return err
	}
	request.Header.Set(handoff.HeaderName, session.secret)
	response, err := e.client().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: session refused with status %d", ErrUnproven, response.StatusCode)
	}
	var answer struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxRemoteAnswer)).Decode(&answer); err != nil || answer.CSRFToken == "" {
		return fmt.Errorf("%w: the session answer is unreadable", ErrUnproven)
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == sessionCookieName {
			session.cookie = cookie.Value
		}
	}
	if session.cookie == "" {
		return fmt.Errorf("%w: the session answer has no cookie", ErrUnproven)
	}
	session.csrf = answer.CSRFToken
	return nil
}

// maxRemoteAnswer は、接続先のエンジンの /cli/ の答えを読む上限である。答えは短い JSON だけである。
const maxRemoteAnswer = 64 << 10

// Status は、接続先のエンジンの状態である。
type Status struct {
	Version      string `json:"version"`
	Vault        bool   `json:"vault"`
	Unlocked     bool   `json:"unlocked"`
	Passwordless bool   `json:"passwordless"`
	Sessions     int    `json:"sessions"`
}

// status は、接続先のエンジンに状態を尋ねる。sshc status と同じ問い合わせである。
func (e *remoteEngine) status(ctx context.Context) (Status, error) {
	session, err := e.current(ctx)
	if err != nil {
		return Status{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, session.origin()+cliStatusPath, nil)
	if err != nil {
		return Status{}, err
	}
	request.Header.Set(handoff.HeaderName, session.secret)
	response, err := e.client().Do(request)
	if err != nil {
		return Status{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		// 接続先のエンジンが起動し直して秘密が変わった。次に受け取り直す。
		e.forget(session)
		return Status{}, ErrNotRunning
	}
	if response.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("remote status answered %d", response.StatusCode)
	}
	var status Status
	if err := json.NewDecoder(io.LimitReader(response.Body, maxRemoteAnswer)).Decode(&status); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (e *remoteEngine) close() {
	e.transport.CloseIdleConnections()
}
