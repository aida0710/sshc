package remoteengine

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sshc/internal/handoff"
)

// forwardedFileRequestHeaders は、ファイルの中身を運ぶ要求のうち、接続先へ渡すヘッダーである。
// 途中からの取得（Range、If-Range）と、本文の種類だけを渡す。cookie と CSRF のトークンは
// このマシンのもので、渡さない。
var forwardedFileRequestHeaders = []string{"Content-Type", "Range", "If-Range"}

// forwardedFileResponseHeaders は、ファイルの中身を運ぶ答えのうち、ブラウザへ渡すヘッダーである。
// 長さと範囲、保存するときの名前（Content-Disposition）、版（ETag）を足す。接続先の cookie や
// セキュリティのヘッダーは渡さない。Content-Security-Policy と X-Content-Type-Options は、
// このマシンのエンジンがどの答えにも付ける（httpserver の Security）。
var forwardedFileResponseHeaders = []string{
	"Content-Type", "Cache-Control", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag",
}

// credentialProbePath は、資格が今も通るかを確かめるのに読む、接続先の軽い操作である。Vault の状態は、
// ロック中でも読める。
const credentialProbePath = "/api/v1/passwords"

// ForwardFileAPI は、ファイルの中身を運ぶ操作を、接続先のエンジンの remotePath へ取り次ぐ。要求の本文も
// 答えの本文も、溜めずに流す（アップロードの一片は数十 MB、ダウンロードは何 GB にもなる）。
//
// 答えを待つ上限（slowForwardedAnswerLimit）は、答えのヘッダーが届くまでに掛ける。本文は、ブラウザが
// 切るまで流し続ける。
//
// 本文は流すと送り直せないので、本文のある要求では、先に軽い操作で資格を確かめる（古い資格は
// そこで受け取り直す）。本文の無い要求は、ForwardAPI と同じく、資格を断られたら一度だけ送り直す。
func (m *Manager) ForwardFileAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error {
	return m.forwardFile(w, r, fileForward{alias: alias, remotePath: remotePath})
}

// ForwardMediaAPI は、<audio> と <video> が読むメディアを取り次ぐ。ブラウザは、この答えを CSRF のトークン無しで
// 取りに来られ、リンクからも開ける（httpserver の forwardedMediaRoute）。接続先が HTML やスクリプトを返すと、
// このマシンの画面のオリジンで開いてしまうので、音声か動画の答えだけを渡す。このマシンのメディアの受け口は
// 自分で種類を決めるが、取り次ぎでは接続先の言う種類を確かめるしかない。
func (m *Manager) ForwardMediaAPI(w http.ResponseWriter, r *http.Request, alias, remotePath string) error {
	return m.forwardFile(w, r, fileForward{alias: alias, remotePath: remotePath, mediaOnly: true})
}

// fileForward は、ファイルの中身を運ぶ取り次ぎの先である。mediaOnly なら、音声か動画の答えだけを渡す。
type fileForward struct {
	alias      string
	remotePath string
	mediaOnly  bool
}

func (m *Manager) forwardFile(w http.ResponseWriter, r *http.Request, to fileForward) error {
	alias, remotePath := to.alias, to.remotePath
	engine, err := m.engine(alias)
	if err != nil {
		return err
	}
	hasBody := r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0
	if hasBody {
		probe := exchangeOf{call: APICall{Method: http.MethodGet, Path: credentialProbePath}, answerLimit: m.answerLimit}
		err := m.exchange(r.Context(), engine, probe, func(response *http.Response) { _ = response.Body.Close() })
		if refused := writeRefusal(w, err); refused || err != nil {
			return ignoringRefusal(err)
		}
	}
	for attempt := 0; ; attempt++ {
		retry, err := m.forwardFileOnce(w, r, engine, fileAttempt{remotePath: remotePath, mayRetry: attempt == 0 && !hasBody, mediaOnly: to.mediaOnly})
		if retry {
			continue
		}
		if refused := writeRefusal(w, err); refused {
			return nil
		}
		return err
	}
}

// fileAttempt は、ファイルの中身を運ぶ要求の1回分である。mayRetry は、資格を断られたら送り直してよいか。
type fileAttempt struct {
	remotePath string
	mayRetry   bool
	mediaOnly  bool
}

func (m *Manager) forwardFileOnce(w http.ResponseWriter, r *http.Request, engine *remoteEngine, attempt fileAttempt) (retry bool, _ error) {
	session, err := engine.current(r.Context())
	if err != nil {
		return false, err
	}
	// 上限は答えのヘッダーが届くまで。届いたら外し、本文はブラウザの要求が終わるまで流す。
	sending, stop := context.WithCancel(r.Context())
	defer stop()
	gaveUp := time.AfterFunc(m.slowAnswerLimit, stop)
	target := url.URL{Scheme: "http", Host: loopbackHost(session.port), Path: attempt.remotePath, RawQuery: r.URL.RawQuery}
	request, _ := http.NewRequestWithContext(sending, r.Method, target.String(), r.Body)
	request.ContentLength = r.ContentLength
	copyAllowedHeaders(request.Header, r.Header, forwardedFileRequestHeaders)
	authorizeRemoteRequest(request, session)
	if token := r.Header.Get(handoff.ActionHeader); token != "" {
		request.Header.Set(handoff.ActionHeader, token)
	}
	response, err := session.client().Do(request)
	headersInTime := gaveUp.Stop()
	if err != nil {
		if !headersInTime && r.Context().Err() == nil {
			engine.forget(session)
			engine.link.resetIfSilent()
		} else if isDialFailure(err) {
			engine.forget(session)
			if attempt.mayRetry {
				return true, nil
			}
		}
		if errors.Is(err, ErrUnproven) {
			return false, ErrUnproven
		}
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: "remote_engine_unreachable"}
	}
	if response.StatusCode == http.StatusUnauthorized && attempt.mayRetry {
		_ = response.Body.Close()
		engine.forget(session)
		return true, nil
	}
	if code := remoteRefusal(response); code != "" {
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: code}
	}
	defer func() { _ = response.Body.Close() }()
	if attempt.mediaOnly && !isMediaAnswer(response) {
		return false, &RemoteRefusalError{Status: http.StatusBadGateway, Code: "remote_engine_refused"}
	}
	copyAllowedHeaders(w.Header(), response.Header, forwardedFileResponseHeaders)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(flushingWriter{writer: w, controller: http.NewResponseController(w)}, response.Body)
	return false, nil
}

// isMediaAnswer は、答えが音声か動画の中身かどうかを返す。うまくいった答え（200 と、途中からの 206）だけを見る。
// 断りは remoteRefusal が先に確かめていて、ここへ来るのは problem の JSON か、条件に合わない範囲の答えである。
func isMediaAnswer(response *http.Response) bool {
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return strings.HasPrefix(response.Header.Get("Content-Type"), "application/problem+json") || response.Header.Get("Content-Type") == ""
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return err == nil && (strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/"))
}

// flushingWriter は、書くたびにブラウザへ送り出す。メディアの再生や進み具合の表示が、溜まった分を待たない。
type flushingWriter struct {
	writer     io.Writer
	controller *http.ResponseController
}

func (f flushingWriter) Write(chunk []byte) (int, error) {
	written, err := f.writer.Write(chunk)
	_ = f.controller.Flush()
	return written, err
}

// writeRefusal は、err が接続先の断りなら、それを答えとして書いて true を返す。
func writeRefusal(w http.ResponseWriter, err error) bool {
	var refusal *RemoteRefusalError
	if !errors.As(err, &refusal) {
		return false
	}
	writeProblem(w, refusal.Status, refusal.Code)
	return true
}

// ignoringRefusal は、答えとして書き終えた断りを、呼び出し側へのエラーにしない。
func ignoringRefusal(err error) error {
	var refusal *RemoteRefusalError
	if errors.As(err, &refusal) {
		return nil
	}
	return err
}
