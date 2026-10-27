package remoteengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// maxForwardedBody は、取り次ぐターミナルの API の要求の本文の上限である。取り次ぐのは
// セッションの作成や名前の変更のような短い JSON だけで、接続先のエンジン自身の上限より
// 小さい。本文は、資格を受け取り直して一度だけ送り直すために読み切って持つ。
const maxForwardedBody = 1 << 20

// RemoteStreamPath は、接続先のエンジンのターミナルの入出力の WebSocket である。
const RemoteStreamPath = "/terminal/stream"

// forwardedAnswerLimit は、取り次いだ要求に接続先のエンジンが答え終えるまでの上限である。
// 取り次ぐのはセッションの一覧や作成のような、すぐ答える要求だけである（作成も SSH の接続を
// 待たずに返る）。上限が無いと、黙って切れた SSH の接続の上で、画面が何も出さずに待ち続ける。
const forwardedAnswerLimit = 20 * time.Second

// ErrRequestTooLarge は、取り次ぐ要求の本文が maxForwardedBody を超えたことを表す。
var ErrRequestTooLarge = errors.New("the request body is too large to forward")

// forwardedResponseHeaders は、接続先の答えのうち、ブラウザへ渡すヘッダーである。cookie
// （接続先の資格）やセキュリティのヘッダー（このマシンのエンジンが付ける）は渡さない。
// Set-Cookie や Clear-Site-Data を渡すと、接続先がこのマシンの画面のセッションを上書き
// したり消したりできる。
var forwardedResponseHeaders = []string{"Content-Type", "Cache-Control"}

// forwardedUpgradeHeaders は、ターミナルの入出力の WebSocket のハンドシェイクの答えの
// うち、ブラウザへ渡すヘッダーである。ハンドシェイクを終えるのに要るものだけを渡す。
var forwardedUpgradeHeaders = []string{
	"Connection", "Upgrade", "Sec-WebSocket-Accept", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions",
}

// ForwardTerminalAPI は、ブラウザの要求を、接続先のエンジンの remotePath（/api/v1/terminal/…）
// へ取り次ぐ。ブラウザの cookie と CSRF のトークンはこのマシンのエンジンのもので、接続先へは
// 送らない。代わりに、このマシンのエンジンが接続先から受け取った資格を付ける。
//
// 資格を用意できなかったときは、何も書かずにその理由を返す（呼び出し側が断りの形にする）。
// 接続先が資格を断ったとき（資格が古くなった）は、受け取り直して一度だけ送り直す。
// 前のポートで接続先のエンジンが身元を示せないとき（起動し直してポートを移った）も、
// handoff を読み直して一度だけ送り直す。
func (m *Manager) ForwardTerminalAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error {
	engine, err := m.engine(alias)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxForwardedBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxForwardedBody {
		return ErrRequestTooLarge
	}
	for attempt := 0; ; attempt++ {
		session, err := engine.current(r.Context())
		if err != nil {
			return err
		}
		answerContext, stopWaiting := context.WithTimeout(r.Context(), m.answerLimit)
		defer stopWaiting()
		response, err := session.client().Do(forwardedRequest(answerContext, r, session, remotePath, body))
		if err != nil && r.Context().Err() == nil && errors.Is(answerContext.Err(), context.DeadlineExceeded) {
			// 接続先が答えない。SSH の接続ごと捨て、次の要求で繋ぎ直す。
			engine.forget(session)
			engine.link.reset()
			writeProblem(w, http.StatusBadGateway, "remote_engine_unreachable")
			return nil
		}
		if err != nil {
			if isDialFailure(err) {
				engine.forget(session)
				if attempt == 0 {
					continue
				}
			}
			if errors.Is(err, ErrUnproven) {
				return ErrUnproven
			}
			writeProblem(w, http.StatusBadGateway, "remote_engine_unreachable")
			return nil
		}
		if response.StatusCode == http.StatusUnauthorized && attempt == 0 {
			_ = response.Body.Close()
			engine.forget(session)
			continue
		}
		if code := remoteRefusal(response); code != "" {
			writeProblem(w, http.StatusBadGateway, code)
			return nil
		}
		copyResponse(w, response)
		return nil
	}
}

// maxRefusalBody は、接続先の断りの本文を読む上限である。断りは短い JSON だけである。
const maxRefusalBody = 64 << 10

// remoteRefusal は、接続先のエンジン自身の資格や Vault についての断りを、このマシンの
// 画面が自分のものと取り違えない code に置き換える。置き換えないものは空を返し、本文は
// 読まずに残す。
//
// 接続先の 401・403 をそのまま返すと、このマシンの画面は自分のセッションが切れたと
// 読んでサインインし直させる。接続先の vault_locked をそのまま返すと、このマシンの
// Vault がロックされたと読んでロックの画面を出す。画面は状態コードに関係なく code で
// 読むので、code も状態コードに関係なく置き換える。
func remoteRefusal(response *http.Response) string {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		_ = response.Body.Close()
		return "remote_engine_refused"
	}
	if response.StatusCode < http.StatusBadRequest {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusalBody))
	_ = response.Body.Close()
	var refusal struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &refusal) == nil {
		if replaced, found := remoteRefusalCodes[refusal.Code]; found {
			return replaced
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return ""
}

// remoteRefusalCodes は、このマシンの画面が自分のセッションや Vault の状態として読む
// code と、接続先のものとして置き換える code である。
var remoteRefusalCodes = map[string]string{
	"vault_locked":     "remote_engine_vault_locked",
	"session_required": "remote_engine_refused",
	"invalid_session":  "remote_engine_refused",
	"invalid_csrf":     "remote_engine_refused",
}

func forwardedRequest(ctx context.Context, r *http.Request, session *remoteSession, remotePath string, body []byte) *http.Request {
	target := url.URL{Scheme: "http", Host: loopbackHost(session.port), Path: remotePath, RawQuery: r.URL.RawQuery}
	request, _ := http.NewRequestWithContext(ctx, r.Method, target.String(), bytes.NewReader(body))
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	authorizeRemoteRequest(request, session)
	return request
}

// authorizeRemoteRequest は、接続先のエンジンの API への要求に、このマシンのエンジンが
// 受け取った資格を付ける。
func authorizeRemoteRequest(request *http.Request, session *remoteSession) {
	request.Header.Set("Accept", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.cookie})
	request.Header.Set(csrfHeaderName, session.csrf)
	// 接続先のエンジンは、同じページからの要求だけを受け付ける。このマシンのエンジンは
	// ブラウザからの要求をこのマシンの規則で確かめ終えているので、接続先の規則を満たして送る。
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("Origin", session.origin())
}

func copyResponse(w http.ResponseWriter, response *http.Response) {
	defer func() { _ = response.Body.Close() }()
	copyAllowedHeaders(w.Header(), response.Header, forwardedResponseHeaders)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
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
	allowed := forwardedResponseHeaders
	if response.StatusCode == http.StatusSwitchingProtocols {
		allowed = forwardedUpgradeHeaders
	}
	kept := http.Header{}
	copyAllowedHeaders(kept, response.Header, allowed)
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
