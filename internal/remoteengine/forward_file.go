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

// forwardedFileResponseHeaders は、ファイルの中身を運ぶ答えのうち、ブラウザへそのまま渡すヘッダーである。
// 長さと範囲、保存するときの名前（Content-Disposition）、版（ETag）を足す。接続先の cookie や
// セキュリティのヘッダーは渡さない。Content-Security-Policy と X-Content-Type-Options は、
// このマシンのエンジンがどの答えにも付ける（httpserver の Security）。Content-Type は、このマシンが
// 1つに決めて書く（copyAnswerHeaders）。
var forwardedFileResponseHeaders = []string{
	"Cache-Control", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag",
}

// forwardedMediaResponseHeaders は、音声と動画の答えのうち、ブラウザへそのまま渡すヘッダーである。
// 途中から再生するための長さと範囲、版だけを渡す。種類、Content-Disposition、Cache-Control は、
// このマシンのメディアの受け口（httpserver の StreamMedia）と同じものをこのマシンが書く（handOverMedia）。
var forwardedMediaResponseHeaders = []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag"}

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
// 自分で種類を決めるが、取り次ぎでは接続先の言う種類を確かめるしかない。確かめた種類は、接続先の
// ヘッダーを写さずにこのマシンが書く（handOverMedia）。
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
	if attempt.mediaOnly {
		return false, handOverMedia(w, response)
	}
	copyAnswerHeaders(w.Header(), response.Header, forwardedFileResponseHeaders)
	w.WriteHeader(response.StatusCode)
	streamBody(w, response.Body)
	return false, nil
}

// handOverMedia は、音声か動画の答えだけをブラウザへ渡す。種類と Content-Disposition は、接続先の
// ヘッダーを写さず、このマシンのメディアの受け口（httpserver の StreamMedia）と同じ形でこのマシンが書く。
// 渡さない答えは、本文を流さずに断りにする（mediaRefusal）。
func handOverMedia(w http.ResponseWriter, response *http.Response) error {
	mediaType := mediaTypeOf(response)
	if mediaType == "" {
		return mediaRefusal(response)
	}
	header := w.Header()
	copyAllowedHeaders(header, response.Header, forwardedMediaResponseHeaders)
	header.Set("Content-Type", mediaType)
	header.Set("Content-Disposition", "inline")
	w.WriteHeader(response.StatusCode)
	streamBody(w, response.Body)
	return nil
}

// mediaTypeOf は、うまくいった答え（200 と、途中からの 206）が音声か動画なら、その種類を返す。
// そうでなければ空を返す。種類は、ちょうど1つ読める形で書かれていなければならない（contentTypeOf）。
func mediaTypeOf(response *http.Response) string {
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentTypeOf(response.Header))
	if err != nil || !(strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/")) {
		return ""
	}
	return mediaType
}

// mediaRefusal は、渡さないメディアの答えを、このマシンの断りにする。
//
// 接続先の断り（400 番台と 500 番台）は、状態コードと code を保つ（チケットの切れた sftp_not_found など）。
// 断りの code は remoteRefusal が先に置き換えてあり、ここへ来るのは、このマシンの画面が自分のものと
// 取り違えない code だけである。code を読めない断りと、断り以外の状態（203 などの 2xx、3xx）は、
// remote_engine_refused にする。2xx の code は remoteRefusal が置き換えないので、読むと、このマシンの画面が
// 自分の Vault のロックと取り違える vault_locked なども届いてしまう。
func mediaRefusal(response *http.Response) error {
	isRemoteProblem := response.StatusCode >= http.StatusBadRequest && response.StatusCode <= maxProblemStatus
	if isRemoteProblem {
		body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusalBody))
		if code := problemCode(body); code != "" {
			return &RemoteRefusalError{Status: response.StatusCode, Code: code}
		}
	}
	return &RemoteRefusalError{Status: http.StatusBadGateway, Code: "remote_engine_refused"}
}

// maxProblemStatus は、断りとして保つ状態コードの上限である。HTTP の状態コードは 599 までだが、
// Go の client は 999 までの3桁を通す。
const maxProblemStatus = 599

// streamBody は、答えの本文を、書くたびにブラウザへ送り出しながら流す。
func streamBody(w http.ResponseWriter, body io.Reader) {
	_, _ = io.Copy(flushingWriter{writer: w, controller: http.NewResponseController(w)}, body)
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
