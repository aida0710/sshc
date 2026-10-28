package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/netaccept"
	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
)

// 実行先のセッションのポート転送（local と dynamic）。
//
// 利用者がつなぐのはこのマシンなので、待ち受けはこのマシンの 127.0.0.1 に開く。実行先のエンジンには、
// 実行先の 127.0.0.1 の空いたポートで同じ転送を開かせ、このマシンの待ち受けに来た接続を、取り次ぎに
// 使っている SSH の接続の上で、そのポートへつなぐ。画面に出す待ち受けのアドレスは、このマシンの
// 待ち受けである。実行先のセッションは実行先の SSH の設定で開いているので、転送を接続先の設定へ
// 保存する選択は無い（その場かぎりの転送だけ）。
//
// remote の転送（接続先で待ち受ける）は、まだ取り次がない。

// runTargetForwardConnectionLimit は、1つの転送の待ち受けが同時に中継する接続の数の上限である。
// このマシンのローカルの転送（sshclient）と同じ値にする。
const runTargetForwardConnectionLimit = 64

// runTargetAnyFreePort は、実行先のエンジンに空いているポートで待ち受けさせる指定である。
const runTargetAnyFreePort = 0

type runTargetForwardKey struct {
	alias     string
	sessionID string
	forwardID string
}

// runTargetForwardRelays は、実行先のセッションの転送ごとの、このマシンの待ち受けの一覧である。
type runTargetForwardRelays struct {
	mutex     sync.Mutex
	listeners map[runTargetForwardKey]net.Listener
}

func newRunTargetForwardRelays() *runTargetForwardRelays {
	return &runTargetForwardRelays{listeners: map[runTargetForwardKey]net.Listener{}}
}

func (r *runTargetForwardRelays) add(key runTargetForwardKey, listener net.Listener) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.listeners[key] = listener
}

// remove は key の中継を一覧から外して、待ち受けを閉じる。
func (r *runTargetForwardRelays) remove(key runTargetForwardKey) {
	r.mutex.Lock()
	listener := r.listeners[key]
	delete(r.listeners, key)
	r.mutex.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
}

func (r *runTargetForwardRelays) has(alias string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for key := range r.listeners {
		if key.alias == alias {
			return true
		}
	}
	return false
}

// localListen は、key の中継のこのマシンの待ち受けのアドレスを返す。
func (r *runTargetForwardRelays) localListen(key runTargetForwardKey) (string, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	listener, ok := r.listeners[key]
	if !ok {
		return "", false
	}
	return listener.Addr().String(), true
}

// keepOnly は、実行先の今のセッションの一覧に無くなった転送（セッションが閉じた、実行先のエンジンが
// 起動し直した）の待ち受けを閉じる。present が nil なら、その実行先の待ち受けをすべて閉じる。
func (r *runTargetForwardRelays) keepOnly(alias string, present map[runTargetForwardKey]bool) {
	r.mutex.Lock()
	var gone []net.Listener
	for key, listener := range r.listeners {
		if key.alias == alias && !present[key] {
			gone = append(gone, listener)
			delete(r.listeners, key)
		}
	}
	r.mutex.Unlock()
	for _, listener := range gone {
		_ = listener.Close()
	}
}

var errRunTargetForwardMissing = errors.New("the run target did not report the forward it opened")

// StartRunTargetForward は、実行先のセッションに転送を開き、このマシンの待ち受けからつなぐ。
func (h RemoteEngineHandlers) StartRunTargetForward(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	sessionID, ok := identifierParam(c, "id")
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var request api.StartTerminalForwardRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	destination := stringValue(request.Destination)
	if request.Kind == "remote" {
		return problem(c, http.StatusConflict, "run_target_remote_forward_unavailable")
	}
	if (request.Kind != "local" && request.Kind != "dynamic") || request.ListenPort < 1 || request.ListenPort > 65535 ||
		len(destination) > 512 || (request.Kind == "local" && destination == "") || (request.Kind == "dynamic" && destination != "") {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(sshclient.LoopbackHost, strconv.Itoa(request.ListenPort)))
	if err != nil {
		return problemWith(c, http.StatusConflict, problemPayload{
			Code: "terminal_forward_bind_failed", Reason: sshclient.ListenProblem(err), Detail: boundedProblemDetail(err.Error()),
		})
	}
	before, err := h.runTargetForwardIDs(c.Request().Context(), alias, sessionID)
	if err != nil {
		_ = listener.Close()
		return remoteEngineProblem(c, err)
	}
	remoteRequest := api.StartTerminalForwardRequest{Kind: request.Kind, ListenPort: runTargetAnyFreePort, Destination: request.Destination}
	body, err := json.Marshal(remoteRequest)
	if err != nil {
		_ = listener.Close()
		return unexpectedProblem(c, "terminal_forward_failed", err)
	}
	reply, err := h.Engines.CallTerminalAPI(c.Request().Context(), alias, remoteengine.TerminalCall{
		Method: http.MethodPost, Path: "/api/v1/terminal/sessions/" + sessionID + "/forwards",
		ContentType: echo.MIMEApplicationJSON, Body: body,
	})
	if err != nil {
		_ = listener.Close()
		return remoteEngineProblem(c, err)
	}
	if reply.Status != http.StatusCreated {
		_ = listener.Close()
		return c.Blob(reply.Status, "application/problem+json", reply.Body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal(reply.Body, &listed); err != nil {
		_ = listener.Close()
		return unexpectedProblem(c, "terminal_forward_failed", err)
	}
	opened, remotePort, found := openedRunTargetForward(listed, sessionID, before)
	if !found {
		_ = listener.Close()
		return unexpectedProblem(c, "terminal_forward_failed", errRunTargetForwardMissing)
	}
	key := runTargetForwardKey{alias: alias, sessionID: sessionID, forwardID: opened}
	h.forwards.add(key, listener)
	go netaccept.Serve(listener, runTargetForwardConnectionLimit, func(local net.Conn) {
		h.relayRunTargetConnection(alias, remotePort, local)
	})
	return c.JSON(http.StatusCreated, h.withLocalListens(alias, listed))
}

// StopRunTargetForward は、このマシンの待ち受けを閉じ、実行先の転送も止める。
func (h RemoteEngineHandlers) StopRunTargetForward(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	sessionID, sessionOK := identifierParam(c, "id")
	forwardID, forwardOK := identifierParam(c, "forwardId")
	if !sessionOK || !forwardOK {
		return problem(c, http.StatusNotFound, "terminal_forward_not_found")
	}
	h.forwards.remove(runTargetForwardKey{alias: alias, sessionID: sessionID, forwardID: forwardID})
	reply, err := h.Engines.CallTerminalAPI(c.Request().Context(), alias, remoteengine.TerminalCall{
		Method: http.MethodDelete, Path: "/api/v1/terminal/sessions/" + sessionID + "/forwards/" + forwardID,
	})
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	if reply.Status != http.StatusOK {
		return c.Blob(reply.Status, "application/problem+json", reply.Body)
	}
	var listed api.TerminalSessionList
	if err := json.Unmarshal(reply.Body, &listed); err != nil {
		return unexpectedProblem(c, "terminal_forward_failed", err)
	}
	return c.JSON(http.StatusOK, h.withLocalListens(alias, listed))
}

// runTargetForwardIDs は、実行先のセッションに今ある転送の ID を返す。
func (h RemoteEngineHandlers) runTargetForwardIDs(ctx context.Context, alias, sessionID string) (map[string]bool, error) {
	reply, err := h.Engines.CallTerminalAPI(ctx, alias, remoteengine.TerminalCall{Method: http.MethodGet, Path: "/api/v1/terminal/sessions"})
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	var listed api.TerminalSessionList
	if reply.Status != http.StatusOK || json.Unmarshal(reply.Body, &listed) != nil {
		return ids, nil
	}
	for _, session := range listed.Sessions {
		if session.Id != sessionID || session.Forwards == nil {
			continue
		}
		for _, forward := range *session.Forwards {
			ids[forward.Id] = true
		}
	}
	return ids, nil
}

// openedRunTargetForward は、一覧のうち before に無かった sessionID の転送と、その実行先のポートを返す。
func openedRunTargetForward(listed api.TerminalSessionList, sessionID string, before map[string]bool) (string, int, bool) {
	for _, session := range listed.Sessions {
		if session.Id != sessionID || session.Forwards == nil {
			continue
		}
		for _, forward := range *session.Forwards {
			if before[forward.Id] || forward.Problem != "" {
				continue
			}
			_, port, err := net.SplitHostPort(forward.Listen)
			number, convErr := strconv.Atoi(port)
			if err != nil || convErr != nil || number < 1 || number > 65535 {
				continue
			}
			return forward.Id, number, true
		}
	}
	return "", 0, false
}

// relayRunTargetConnection は、このマシンの待ち受けに来た接続を、実行先の 127.0.0.1:remotePort へつなぐ。
func (h RemoteEngineHandlers) relayRunTargetConnection(alias string, remotePort int, local net.Conn) {
	defer func() { _ = local.Close() }()
	remote, err := h.Engines.DialRunTargetLoopback(context.Background(), alias, remotePort)
	if err != nil {
		return
	}
	defer func() { _ = remote.Close() }()
	done := make(chan struct{}, 2)
	copyHalf := func(to, from net.Conn) {
		_, _ = io.Copy(to, from)
		if half, ok := to.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}
		done <- struct{}{}
	}
	go copyHalf(remote, local)
	go copyHalf(local, remote)
	<-done
	<-done
}

// withLocalListens は、実行先のセッションの一覧の転送の待ち受けのアドレスを、このマシンの待ち受けに
// 替える。実行先のポートは、利用者がつなぐ先ではないからである。一覧は実行先のすべてのセッションを
// 含むので、一覧に無い転送の待ち受けは閉じる。
func (h RemoteEngineHandlers) withLocalListens(alias string, listed api.TerminalSessionList) api.TerminalSessionList {
	present := map[runTargetForwardKey]bool{}
	for index := range listed.Sessions {
		session := &listed.Sessions[index]
		if session.Forwards == nil {
			continue
		}
		for forwardIndex := range *session.Forwards {
			forward := &(*session.Forwards)[forwardIndex]
			key := runTargetForwardKey{alias: alias, sessionID: session.Id, forwardID: forward.Id}
			present[key] = true
			if local, ok := h.forwards.localListen(key); ok {
				forward.Listen = local
			}
		}
	}
	h.forwards.keepOnly(alias, present)
	return listed
}

// forwardWithLocalListens は、このマシンが中継している転送のある実行先へ、ブラウザの要求を取り次ぐ。
// 答えがセッションの一覧なら、転送の待ち受けのアドレスをこのマシンの待ち受けに替えて返す。
func (h RemoteEngineHandlers) forwardWithLocalListens(c *echo.Context, alias, remotePath string) error {
	request := c.Request()
	body, err := io.ReadAll(io.LimitReader(request.Body, maxForwardedRequestBody+1))
	if err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(body) > maxForwardedRequestBody {
		// 取り次ぎ（remoteengine）が大きすぎる本文を断るときと同じ答えにする。
		return remoteEngineProblem(c, remoteengine.ErrRequestTooLarge)
	}
	reply, err := h.Engines.CallTerminalAPI(request.Context(), alias, remoteengine.TerminalCall{
		Method: request.Method, Path: remotePath, RawQuery: request.URL.RawQuery,
		ContentType: request.Header.Get(echo.HeaderContentType), Body: body,
	})
	if err != nil {
		return remoteEngineProblem(c, err)
	}
	var listed api.TerminalSessionList
	if reply.Status >= http.StatusOK && reply.Status < http.StatusMultipleChoices &&
		json.Unmarshal(reply.Body, &listed) == nil && listed.Sessions != nil {
		return c.JSON(reply.Status, h.withLocalListens(alias, listed))
	}
	contentType := reply.ContentType
	if contentType == "" {
		contentType = echo.MIMEApplicationJSON
	}
	if len(reply.Body) == 0 {
		return c.NoContent(reply.Status)
	}
	return c.Blob(reply.Status, contentType, reply.Body)
}

// maxForwardedRequestBody は、中継のある実行先へ取り次ぐ要求の本文の上限である。取り次ぎ
// （remoteengine）と同じく、セッションの作成や名前の変更のような短い JSON だけを運ぶ。
const maxForwardedRequestBody = 1 << 20
