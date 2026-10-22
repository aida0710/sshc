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
)

// maxForwardedBody は、取り次ぐターミナルの API の要求の本文の上限である。取り次ぐのは
// セッションの作成や名前の変更のような短い JSON だけで、接続先のエンジン自身の上限より
// 小さい。本文は、資格を受け取り直して一度だけ送り直すために読み切って持つ。
const maxForwardedBody = 1 << 20

// RemoteStreamPath は、接続先のエンジンのターミナルの入出力の WebSocket である。
const RemoteStreamPath = "/terminal/stream"

// ErrRequestTooLarge は、取り次ぐ要求の本文が maxForwardedBody を超えたことを表す。
var ErrRequestTooLarge = errors.New("the request body is too large to forward")

// forwardedResponseHeaders は、接続先の答えのうち、ブラウザへ渡すヘッダーである。cookie
// （接続先の資格）やセキュリティのヘッダー（このマシンのエンジンが付ける）は渡さない。
var forwardedResponseHeaders = []string{"Content-Type", "Cache-Control"}

// ForwardTerminalAPI は、ブラウザの要求を、接続先のエンジンの remotePath（/api/v1/terminal/…）
// へ取り次ぐ。ブラウザの cookie と CSRF のトークンはこのマシンのエンジンのもので、接続先へは
// 送らない。代わりに、このマシンのエンジンが接続先から受け取った資格を付ける。
//
// 資格を用意できなかったときは、何も書かずにその理由を返す（呼び出し側が断りの形にする）。
// 接続先が資格を断ったとき（資格が古くなった）は、受け取り直して一度だけ送り直す。
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
		response, err := engine.client().Do(forwardedRequest(r.Context(), r, session, remotePath, body))
		if err != nil {
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
// Vault がロックされたと読んでロックの画面を出す。
func remoteRefusal(response *http.Response) string {
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		_ = response.Body.Close()
		return "remote_engine_refused"
	case http.StatusConflict:
		body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusalBody))
		_ = response.Body.Close()
		var refusal struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(body, &refusal) == nil && refusal.Code == "vault_locked" {
			return "remote_engine_vault_locked"
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
	}
	return ""
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
	for _, name := range forwardedResponseHeaders {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
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
		Transport:     engine.transport,
		FlushInterval: -1,
		ErrorHandler: func(response http.ResponseWriter, _ *http.Request, _ error) {
			writeProblem(response, http.StatusBadGateway, "remote_engine_unreachable")
		},
	}
	proxy.ServeHTTP(w, r)
	return nil
}
