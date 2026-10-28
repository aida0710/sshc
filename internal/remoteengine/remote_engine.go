package remoteengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"sshc/internal/handoff"
)

var (
	// ErrNotRunning は、接続先で sshc のエンジンが動いていないことを表す。
	ErrNotRunning = errors.New("the sshc engine on the remote host is not running")
	// ErrUnproven は、接続先のエンジンが handoff の秘密を持つことを示せなかったことを表す。
	ErrUnproven = errors.New("the remote engine could not prove that it owns its handoff")
)

// Compatibility は、実行先とのやり取りの互換の番号である。handoff の
// runTargetCompatibility に書き、接続先の番号がこれと同じなら、sshc のバージョンが
// 違っても実行先として使う。画面は接続先のエンジンの API をそのまま使うためである。
//
// 取り次ぐ API（httpserver の registerRemoteEngineRoutes が接続先へ渡す
// /api/v1/terminal/… の要求と答え）、handoff と /cli/ の手順、ターミナルの入出力の
// ストリームの形を、古い相手と通じない形に変えたときに 1 つ上げる。
// 上げたリリースでは、実行先の sshc も更新が要る（docs/releasing.md）。
// 0 は、番号を書かない古い sshc の handoff を読んだときの値なので使わない。
const Compatibility = 1

// IncompatibleEngineError は、接続先の sshc の互換の番号がこのマシンと違うことを表す。
// 番号を書かない古い sshc も含む。Local と Remote は、それぞれの sshc のバージョンである。
type IncompatibleEngineError struct{ Local, Remote string }

func (e *IncompatibleEngineError) Error() string {
	return fmt.Sprintf("the remote engine runs sshc %s, which is not compatible with this engine's sshc %s", e.Remote, e.Local)
}

// remoteSession は、接続先のエンジンの API を使うための資格である。CLI と同じく
// /cli/session で受け取る。transport は、この資格を受け取ったエンジンへだけ繋ぐ。
type remoteSession struct {
	engineIdentity
	transport *http.Transport
	cookie    string
	csrf      string
}

func (s *remoteSession) origin() string { return "http://" + loopbackHost(s.port) }

func (s *remoteSession) client() *http.Client {
	return &http.Client{
		Transport: s.transport,
		// 接続先のエンジンは redirect を返さない。返されても、秘密や cookie を付けたまま
		// 別の場所へ送らない。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// newSessionTransport は、identity のエンジンへだけ繋ぐ Transport を返す。新しく開く
// TCP の接続ごとに、資格を送る前に相手の身元を確かめる。
func newSessionTransport(link *hostLink, identity engineIdentity) *http.Transport {
	return &http.Transport{
		// 宛先は接続先から見た 127.0.0.1:<identity.port> だけである。要求の URL が別の
		// ポートを指していても、そこへは開かない。
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			if address != loopbackHost(identity.port) {
				return nil, &dialFailure{err: fmt.Errorf("unexpected address %s", address)}
			}
			return link.dialProven(ctx, identity)
		},
		DisableCompression: true,
	}
}

// remoteEngine は、実行先に足した接続先のエンジンひとつである。
type remoteEngine struct {
	link         *hostLink
	localVersion string
	// sessionLimit は、/cli/session の答えを待つ上限（sessionAnswerLimit）である。試験だけが短くする。
	sessionLimit time.Duration
	// attaching は、同時に来た要求に、ひとつの資格の受け取りを待たせる。
	attaching sharedAttempt[*remoteSession]

	mutex   sync.Mutex
	session *remoteSession
	// generation は、資格を捨てるたびに増える。捨てたあとに終わった受け取りを持たない。
	generation uint64
}

func newRemoteEngine(link *hostLink, localVersion string, sessionLimit time.Duration) *remoteEngine {
	engine := &remoteEngine{link: link, localVersion: localVersion, sessionLimit: sessionLimit}
	link.whenEnded(engine.closeIdleConnections)
	return engine
}

// attached は、接続先のエンジンの資格を今持っているかを答える。受け取りには行かない。
func (e *remoteEngine) attached() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.session != nil
}

// current は、使える資格を返す。まだ無ければ、接続先で handoff を読んで受け取る。
//
// 受け取りはロックの外で行い、そのあいだに来た要求は同じ受け取りを待つ。
func (e *remoteEngine) current(ctx context.Context) (*remoteSession, error) {
	e.mutex.Lock()
	session, generation := e.session, e.generation
	e.mutex.Unlock()
	if session != nil {
		return session, nil
	}
	return e.attaching.do(ctx, e.attachLimit(), func(ctx context.Context) (*remoteSession, error) {
		session, err := e.attach(ctx)
		if err != nil {
			return nil, err
		}
		e.mutex.Lock()
		defer e.mutex.Unlock()
		if e.generation == generation {
			e.session = session
		}
		return session, nil
	})
}

// attachLimit は、資格の受け取りひとつの上限である。SSH の接続を待ち、handoff を読み、
// /cli/session の答えを受け取るまでの、各段の上限の和にする。
func (e *remoteEngine) attachLimit() time.Duration {
	return e.link.attemptLimit + handoffLimit + e.sessionLimit
}

// forget は、断られた資格を捨てる。次の要求で受け取り直す。
func (e *remoteEngine) forget(stale *remoteSession) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.session == stale {
		e.session = nil
		e.generation++
		stale.transport.CloseIdleConnections()
	}
}

// forgetAny は、持っている資格を捨てる。エンジンを止めたり起動し直したりしたあとに使う。
func (e *remoteEngine) forgetAny() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.session != nil {
		e.session.transport.CloseIdleConnections()
		e.session = nil
	}
	e.generation++
}

// closeIdleConnections は、待機中の HTTP の接続を捨てる。SSH の接続が終わると、その上の
// 接続は使えない。残すと次の要求がそれを使って失敗する。
func (e *remoteEngine) closeIdleConnections() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.session != nil {
		e.session.transport.CloseIdleConnections()
	}
}

// attach は、接続先の handoff を読み、相手がその秘密を持つエンジンかを確かめてから、
// API の資格を受け取る。手順はこのマシンの sshc CLI と同じで、確かめるのは
// newSessionTransport が開く接続ごとである。
func (e *remoteEngine) attach(ctx context.Context) (*remoteSession, error) {
	identity, err := readIdentity(ctx, e.link)
	if err != nil {
		return nil, err
	}
	if identity.compatibility != Compatibility {
		return nil, &IncompatibleEngineError{Local: e.localVersion, Remote: identity.version}
	}
	session := &remoteSession{engineIdentity: identity, transport: newSessionTransport(e.link, identity)}
	if err := e.openSession(ctx, session); err != nil {
		session.transport.CloseIdleConnections()
		var connectErr *ConnectError
		if isDialFailure(err) && !errors.Is(err, ErrUnproven) && !errors.As(err, &connectErr) {
			// handoff は残っているが、そのポートで誰も待ち受けていない。
			return nil, fmt.Errorf("%w: %v", ErrNotRunning, err)
		}
		return nil, err
	}
	return session, nil
}

// openSession は、handoff の秘密と引き換えに、API の cookie と CSRF のトークンを受け取る。
func (e *remoteEngine) openSession(ctx context.Context, session *remoteSession) error {
	ctx, cancel := context.WithTimeout(ctx, e.sessionLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, session.origin()+handoff.SessionPath, nil)
	if err != nil {
		return err
	}
	request.Header.Set(handoff.HeaderName, session.secret)
	response, err := session.client().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: session refused with status %d", ErrUnproven, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteAnswer))
	if err != nil {
		return err
	}
	credentials, err := handoff.ReadSessionAnswer(body, response.Cookies())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnproven, err)
	}
	session.cookie, session.csrf = credentials.Cookie, credentials.CSRF
	return nil
}

// sessionAnswerLimit は、接続先のエンジンが /cli/session に答え終えるまでの上限である。新しい
// TCP の接続なら身元の確かめ（proofLimit）を含むので、答えそのものにも同じだけ待つ。
const sessionAnswerLimit = 2 * proofLimit

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
// 接続先のエンジンが起動し直して前のポートで身元を示せなければ、handoff を読み直して
// 一度だけ尋ね直す。
func (e *remoteEngine) status(ctx context.Context) (Status, error) {
	for attempt := 0; ; attempt++ {
		session, err := e.current(ctx)
		if err != nil {
			return Status{}, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, session.origin()+handoff.StatusPath, nil)
		if err != nil {
			return Status{}, err
		}
		request.Header.Set(handoff.HeaderName, session.secret)
		response, err := session.client().Do(request)
		if err != nil {
			if isDialFailure(err) {
				e.forget(session)
				if attempt == 0 {
					continue
				}
			}
			return Status{}, err
		}
		return e.readStatus(session, response)
	}
}

func (e *remoteEngine) readStatus(session *remoteSession, response *http.Response) (Status, error) {
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		// 接続先のエンジンが起動し直して秘密が変わった。次に受け取り直す。
		e.forget(session)
		return Status{}, ErrNotRunning
	}
	if response.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("remote status answered %d", response.StatusCode)
	}
	// 接続先は別のバージョンの sshc でありうるので、知らない項目は読み飛ばす。
	var wire handoff.EngineStatusWire
	if err := json.NewDecoder(io.LimitReader(response.Body, maxRemoteAnswer)).Decode(&wire); err != nil {
		return Status{}, err
	}
	answer, err := wire.Status()
	if err != nil {
		return Status{}, err
	}
	return Status{Version: answer.Version, Vault: answer.Vault, Unlocked: answer.Unlocked, Passwordless: answer.Passwordless, Sessions: answer.Sessions}, nil
}

func (e *remoteEngine) close() {
	e.forgetAny()
}
