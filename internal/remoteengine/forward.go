package remoteengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"sshc/internal/handoff"
)

// maxForwardedBody は、本文を溜めて取り次ぐ要求の本文の上限である。このマシンの入口の共通の上限と同じにする。
// 小さいと、このマシンでは受ける本文（大きい SSH の設定の保存など）が、リモートセッションでだけ断られる。
// これより大きい本文を受ける操作は、溜めずに流す（ForwardFileAPI）。本文は、資格を受け取り直して一度だけ
// 送り直すために読み切って持つ。
const maxForwardedBody = handoff.MaxRequestBodyCeiling

// RemoteStreamPath は、接続先のエンジンのターミナルの入出力の WebSocket である。
const RemoteStreamPath = "/terminal/stream"

// forwardedAnswerLimit は、取り次いだ要求に接続先のエンジンが答え終えるまでの上限である。
// 取り次ぐのはセッションの一覧や作成のような、すぐ答える要求だけである（作成も SSH の接続を
// 待たずに返る）。上限が無いと、黙って切れた SSH の接続の上で、画面が何も出さずに待ち続ける。
const forwardedAnswerLimit = 20 * time.Second

// slowForwardedAnswerLimit は、接続先のエンジンが、よそのマシンの答えを待ってから答える操作の上限である。
// 認証の試し、ホスト鍵の読み取り、鍵の登録、VPN の経路の開始がそれで、一番長い VPN の承認待ち（2 分）が
// 収まる長さにする。forwardedAnswerLimit のままだと、接続先がまだ働いているうちに「届かない」と断り、
// 資格を捨ててしまう。
const slowForwardedAnswerLimit = 3 * time.Minute

// ErrRequestTooLarge は、取り次ぐ要求の本文が maxForwardedBody を超えたことを表す。
var ErrRequestTooLarge = errors.New("the request body is too large to forward")

// forwardedResponseHeaders は、接続先の答えのうち、ブラウザへそのまま渡すヘッダーである。cookie
// （接続先の資格）やセキュリティのヘッダー（このマシンのエンジンが付ける）は渡さない。
// Set-Cookie や Clear-Site-Data を渡すと、接続先がこのマシンの画面のセッションを上書き
// したり消したりできる。Content-Type は、このマシンが1つに決めて書く（copyAnswerHeaders）。
var forwardedResponseHeaders = []string{"Cache-Control"}

// unknownContentType は、接続先の答えの種類が1つに決まらないとき（無い、2つ以上ある、読めない）に書く種類である。
// 種類の無い答えには、このマシンの Go の server が本文から判定した text/html などを付ける
// （X-Content-Type-Options: nosniff はブラウザへの指示で、server の判定は止めない）。種類が2つあると、
// ブラウザは後ろの種類で読む。どちらも、中身の分からない答えとして渡す。
const unknownContentType = "application/octet-stream"

// forwardedUpgradeHeaders は、ターミナルの入出力の WebSocket のハンドシェイクの答えの
// うち、ブラウザへ渡すヘッダーである。ハンドシェイクを終えるのに要るものだけを渡す。
var forwardedUpgradeHeaders = []string{
	"Connection", "Upgrade", "Sec-WebSocket-Accept", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions",
}

// ForwardAPI は、ブラウザの要求を、接続先のエンジンの remotePath（/api/v1/…）
// へ取り次ぐ。ブラウザの cookie と CSRF のトークンはこのマシンのエンジンのもので、接続先へは
// 送らない。代わりに、このマシンのエンジンが接続先から受け取った資格を付ける。
//
// 資格を用意できなかったときは、何も書かずにその理由を返す（呼び出し側が断りの形にする）。
// 接続先が資格を断ったとき（資格が古くなった）は、受け取り直して一度だけ送り直す。
// 前のポートで接続先のエンジンが身元を示せないとき（起動し直してポートを移った）も、
// handoff を読み直して一度だけ送り直す。
func (m *Manager) ForwardAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error {
	return m.forward(w, r, forwardTo{alias: alias, remotePath: remotePath, answerLimit: m.answerLimit})
}

// ForwardSlowAPI は ForwardAPI と同じ取り次ぎで、答えを slowForwardedAnswerLimit まで待つ。
func (m *Manager) ForwardSlowAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error {
	return m.forward(w, r, forwardTo{alias: alias, remotePath: remotePath, answerLimit: m.slowAnswerLimit})
}

// forwardTo は、取り次ぐ先と、答えを待つ上限である。
type forwardTo struct {
	alias       string
	remotePath  string
	answerLimit time.Duration
}

func (m *Manager) forward(w http.ResponseWriter, r *http.Request, to forwardTo) error {
	engine, err := m.engine(to.alias)
	if err != nil {
		return err
	}
	call, err := ForwardedCall(r, to.remotePath)
	if err != nil {
		return err
	}
	err = m.exchange(r.Context(), engine, exchangeOf{call: call, answerLimit: to.answerLimit}, func(response *http.Response) { copyResponse(w, response) })
	var refusal *RemoteRefusalError
	if errors.As(err, &refusal) {
		writeProblem(w, refusal.Status, refusal.Code)
		return nil
	}
	return err
}

// ForwardedCall は、ブラウザの要求を、接続先のエンジンの remotePath（/api/v1/…）への要求にする。本文、
// クエリ、本文の種類、確認のトークンを写す。確認のトークンは、接続先のエンジンが出したものを画面が持っていて、
// 接続先が確かめる。ブラウザの cookie と CSRF のトークンはこのマシンのエンジンのもので、写さない。
// 本文が maxForwardedBody を超えれば ErrRequestTooLarge を返す。
func ForwardedCall(r *http.Request, remotePath string) (APICall, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxForwardedBody+1))
	// このマシンの入口（httpserver の Security）は、長さを言わない本文を上限で切る。
	var cut *http.MaxBytesError
	if errors.As(err, &cut) || len(body) > maxForwardedBody {
		return APICall{}, ErrRequestTooLarge
	}
	if err != nil {
		return APICall{}, err
	}
	return APICall{
		Method: r.Method, Path: remotePath, RawQuery: r.URL.RawQuery, ContentType: r.Header.Get("Content-Type"), Body: body,
		ActionToken: r.Header.Get(handoff.ActionHeader),
	}, nil
}

// APICall は、接続先のエンジンの API へ送る要求1つである。このマシンのエンジン自身が組むもの（転送を開く要求
// など）と、ブラウザの要求を写したもの（ForwardedCall）がある。
type APICall struct {
	Method string
	// Path は /api/v1/… である。
	Path        string
	RawQuery    string
	ContentType string
	Body        []byte
	// ActionToken は、接続先のエンジンが出した一度限りの確認のトークンである。あれば送る。
	ActionToken string
}

// APIAnswer は、接続先のエンジンの答えである。
type APIAnswer struct {
	Status int
	// ContentType は、答えの種類をこのマシンが1つに決めたものである（contentTypeOf）。
	ContentType string
	Body        []byte
}

// maxCalledAnswer は、CallAPI が読む答えの上限である。答えは、最大 64 の宛先と
// それぞれのコマンドを並べた JSON か、セッションの一覧（転送を中継している実行先の取り次ぎ）
// までである。
const maxCalledAnswer = 1 << 20

// ErrAnswerTooLarge は、接続先の答えが maxCalledAnswer を超えたことを表す。
var ErrAnswerTooLarge = errors.New("the remote engine's answer is too large")

// CallAPI は、call を接続先のエンジンへ送り、答えを返す。資格の扱いと送り直しは
// ForwardAPI と同じである。接続先へ届かなかったときや、接続先が資格や Vault を
// 断ったときは *RemoteRefusalError を返す。
func (m *Manager) CallAPI(ctx context.Context, alias string, call APICall) (APIAnswer, error) {
	engine, err := m.engine(alias)
	if err != nil {
		return APIAnswer{}, err
	}
	var answer APIAnswer
	var readErr error
	err = m.exchange(ctx, engine, exchangeOf{call: call, answerLimit: m.answerLimit}, func(response *http.Response) {
		defer func() { _ = response.Body.Close() }()
		answer.Status = response.StatusCode
		answer.ContentType = contentTypeOf(response.Header)
		answer.Body, readErr = io.ReadAll(io.LimitReader(response.Body, maxCalledAnswer+1))
		if readErr == nil && len(answer.Body) > maxCalledAnswer {
			readErr = ErrAnswerTooLarge
		}
	})
	if err != nil {
		return APIAnswer{}, err
	}
	return answer, readErr
}

// RemoteRefusalError は、接続先へ届かなかったこと、接続先のエンジンが資格や Vault を断ったこと、
// または接続先の答えをこのマシンが渡さないことを、このマシンの画面へ返す状態コードと語にしたものである。
type RemoteRefusalError struct {
	Status int
	Code   string
}

func (e *RemoteRefusalError) Error() string { return "the remote engine refused: " + e.Code }

// exchange は call を送り、答えを receive に渡す。receive は本文を閉じる。資格が古いか、
// 前のポートに繋がらなかった1回目は、資格を受け取り直して一度だけ送り直す。
func (m *Manager) exchange(ctx context.Context, engine *remoteEngine, asked exchangeOf, receive func(*http.Response)) error {
	for attempt := 0; ; attempt++ {
		retry, err := m.exchangeOnce(ctx, engine, exchangeAttempt{call: asked.call, answerLimit: asked.answerLimit, first: attempt == 0}, receive)
		if !retry {
			return err
		}
	}
}

// exchangeOf は、送る要求と、答えを待つ上限である。
type exchangeOf struct {
	call        APICall
	answerLimit time.Duration
}

// exchangeAttempt は、送る要求の1回分である。first は、断られたら送り直してよい1回目かどうか。
type exchangeAttempt struct {
	call        APICall
	answerLimit time.Duration
	first       bool
}

// exchangeOnce は、要求を1回送る。答えを待つ期限は、receive が本文を読み終えるまでこの1回の中で持つ。
func (m *Manager) exchangeOnce(ctx context.Context, engine *remoteEngine, attempt exchangeAttempt, receive func(*http.Response)) (retry bool, _ error) {
	session, err := engine.current(ctx)
	if err != nil {
		return false, err
	}
	answerContext, stopWaiting := context.WithTimeout(ctx, attempt.answerLimit)
	defer stopWaiting()
	response, err := session.client().Do(attempt.call.request(answerContext, session))
	if err != nil && ctx.Err() == nil && errors.Is(answerContext.Err(), context.DeadlineExceeded) {
		// 接続先が答えない。資格を捨て、SSH の接続も答えなければ捨てて、次の要求で繋ぎ直す。
		engine.forget(session)
		engine.link.resetIfSilent()
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: "remote_engine_unreachable"}
	}
	if err != nil {
		if isDialFailure(err) {
			engine.forget(session)
			if attempt.first {
				return true, nil
			}
		}
		if errors.Is(err, ErrUnproven) {
			return false, ErrUnproven
		}
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: "remote_engine_unreachable"}
	}
	if response.StatusCode == http.StatusUnauthorized && attempt.first {
		_ = response.Body.Close()
		engine.forget(session)
		return true, nil
	}
	if code := remoteRefusal(response); code != "" {
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: code}
	}
	receive(response)
	return false, nil
}

// maxRefusalBody は、接続先の断りの本文を読む上限である。断りは短い JSON だけである。
const maxRefusalBody = 64 << 10

// remoteRefusal は、接続先のエンジン自身の資格や Vault についての断りを、このマシンの
// 画面が自分のものと取り違えない code に置き換える。置き換えないものは空を返し、本文は
// 読まずに残す。
//
// 接続先の 401 と、入口の 403（Host、Origin、CSRF）をそのまま返すと、このマシンの画面は
// 自分のセッションが切れたと読んでサインインし直させる。接続先の vault_locked をそのまま
// 返すと、このマシンの Vault がロックされたと読んでロックの画面を出す。画面は状態コードに
// 関係なく code で読むので、code も状態コードに関係なく置き換える。
//
// 403 のうち、操作そのものの断り（マスターパスワードの間違い wrong_passphrase、確認のトークンの
// 断り action_token_…、編集できない場所 path_not_editable など）は、画面がその code で文を出すので、
// 本文ごとそのまま返す。code を読めない 403 は、入口の断りとして扱う。
func remoteRefusal(response *http.Response) string {
	if response.StatusCode == http.StatusUnauthorized {
		_ = response.Body.Close()
		return "remote_engine_refused"
	}
	// 2xx は画面が成功として読み、code を見ない。それ以外（fetch が追わない 3xx を含む）は
	// 画面が code で読むので、本文を確かめる。
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusalBody))
	_ = response.Body.Close()
	code := problemCode(body)
	if replaced, found := remoteRefusalCodes[code]; found {
		return replaced
	}
	if response.StatusCode == http.StatusForbidden && code == "" {
		return "remote_engine_refused"
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return ""
}

// problemCode は、接続先の断りの本文（problem の JSON）の code を返す。読めなければ空を返す。
func problemCode(body []byte) string {
	var refusal struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &refusal) != nil {
		return ""
	}
	return refusal.Code
}

// remoteRefusalCodes は、このマシンの画面が自分のセッションや Vault の状態として読む
// code と、接続先のものとして置き換える code である。
var remoteRefusalCodes = map[string]string{
	"vault_locked":     "remote_engine_vault_locked",
	"session_required": "remote_engine_refused",
	"invalid_session":  "remote_engine_refused",
	"invalid_csrf":     "remote_engine_refused",
	// 接続先の入口が、このマシンのエンジンからの要求の Host や Origin を断った。
	"invalid_host":       "remote_engine_refused",
	"cross_site_request": "remote_engine_refused",
}

// request は、call を接続先のエンジンへの要求にし、このマシンのエンジンが受け取った資格を付ける。
func (call APICall) request(ctx context.Context, session *remoteSession) *http.Request {
	target := url.URL{Scheme: "http", Host: loopbackHost(session.port), Path: call.Path, RawQuery: call.RawQuery}
	request, _ := http.NewRequestWithContext(ctx, call.Method, target.String(), bytes.NewReader(call.Body))
	if call.ContentType != "" {
		request.Header.Set("Content-Type", call.ContentType)
	}
	authorizeRemoteRequest(request, session)
	if call.ActionToken != "" {
		request.Header.Set(handoff.ActionHeader, call.ActionToken)
	}
	return request
}

// authorizeRemoteRequest は、接続先のエンジンの API への要求に、このマシンのエンジンが
// 受け取った資格を付ける。
func authorizeRemoteRequest(request *http.Request, session *remoteSession) {
	request.Header.Set("Accept", "application/json")
	request.AddCookie(&http.Cookie{Name: handoff.SessionCookie, Value: session.cookie})
	request.Header.Set(handoff.CSRFHeader, session.csrf)
	// 接続先のエンジンは、同じページからの要求だけを受け付ける。このマシンのエンジンは
	// ブラウザからの要求をこのマシンの規則で確かめ終えているので、接続先の規則を満たして送る。
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Origin", session.origin())
}

func copyResponse(w http.ResponseWriter, response *http.Response) {
	defer func() { _ = response.Body.Close() }()
	copyAnswerHeaders(w.Header(), response.Header, forwardedResponseHeaders)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

// copyAnswerHeaders は、接続先の答えのヘッダーのうち allowed に挙げたものを destination へ写し、
// 種類は contentTypeOf で1つに決めて書く。
func copyAnswerHeaders(destination, source http.Header, allowed []string) {
	copyAllowedHeaders(destination, source, allowed)
	destination.Set("Content-Type", contentTypeOf(source))
}

// contentTypeOf は、接続先の答えの種類を、ブラウザへ書く1つの値にして返す。種類がちょうど1つ、読める形で
// 書かれていなければ unknownContentType を返す。1つの値の中にカンマで2つ並べたもの（ブラウザは後ろを使う）も、
// 読めない形として断る。
func contentTypeOf(header http.Header) string {
	declared := header.Values("Content-Type")
	if len(declared) != 1 {
		return unknownContentType
	}
	mediaType, params, err := mime.ParseMediaType(declared[0])
	formatted := mime.FormatMediaType(mediaType, params)
	if err != nil || formatted == "" {
		return unknownContentType
	}
	return formatted
}

// copyAllowedHeaders は、source のヘッダーのうち allowed に挙げたものだけを destination へ
// 写す。source にあるものは、destination の同じ名前のヘッダーを置き換える。
func copyAllowedHeaders(destination, source http.Header, allowed []string) {
	for _, name := range allowed {
		values := source.Values(name)
		if len(values) == 0 {
			continue
		}
		destination.Del(name)
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

// keepStreamHeaders は、接続先の WebSocket の答えのヘッダーを、ブラウザへ渡してよいもの
// だけにする。API の取り次ぎと同じく、接続先の cookie などはこのマシンの画面へ渡さない。
func keepStreamHeaders(response *http.Response) error {
	kept := http.Header{}
	if response.StatusCode == http.StatusSwitchingProtocols {
		copyAllowedHeaders(kept, response.Header, forwardedUpgradeHeaders)
	} else {
		copyAnswerHeaders(kept, response.Header, forwardedResponseHeaders)
	}
	response.Header = kept
	return nil
}

// ForwardTerminalStream は、ターミナルの入出力の WebSocket を、接続先のエンジンへ取り次ぐ。
// 接続先はチケット（接続先が発行した使い捨てのもの）と Origin で確かめるので、Origin を
// 接続先のものに書き換え、ブラウザの cookie は送らない。ブラウザの Origin は、呼び出し側が
// このマシンのものか確かめてから呼ぶ。
func (m *Manager) ForwardTerminalStream(w http.ResponseWriter, r *http.Request, alias string) error {
	engine, err := m.engine(alias)
	if err != nil {
		return err
	}
	session, err := engine.current(r.Context())
	if err != nil {
		return err
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(&url.URL{Scheme: "http", Host: loopbackHost(session.port)})
			request.Out.URL.Path = RemoteStreamPath
			request.Out.URL.RawPath = ""
			request.Out.Header.Del("Cookie")
			request.Out.Header.Set("Origin", session.origin())
		},
		ModifyResponse: keepStreamHeaders,
		Transport:      session.transport,
		FlushInterval:  -1,
		ErrorHandler: func(response http.ResponseWriter, _ *http.Request, err error) {
			// チケットは前のエンジンが発行したものなので、ここでは送り直さない。資格だけを
			// 捨て、画面が繋ぎ直すときに handoff を読み直す。
			if isDialFailure(err) {
				engine.forget(session)
			}
			writeProblem(response, http.StatusBadGateway, unreachableCode(err))
		},
	}
	proxy.ServeHTTP(w, r)
	return nil
}

// unreachableCode は、接続先のエンジンへ届かなかった理由の code である。
func unreachableCode(err error) string {
	if errors.Is(err, ErrUnproven) {
		return "remote_engine_unproven"
	}
	return "remote_engine_unreachable"
}
