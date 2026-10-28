package httpserver

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remoteengine"
	"sshc/internal/session"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
)

// 実行先のセッションへのクイックコマンドと一括送信。
//
// スニペットはこのマシンのもの（画面に出ている一覧）で、展開（変数・シークレット）もこのマシンの
// エンジンでする。ブラウザに見せるのは、このマシンの伏せた形（PreparedCommand.Display）だけである。
// 展開した本文は、実行先のエンジンへだけ送る。
//
// 確認のトークンは2段である。ブラウザのトークンはこのマシンのエンジンが出し、このマシンの証拠
// （展開したコマンド・宛先・Enter・実行先）に結び付ける。実行先のエンジンは、取り次いだ「展開した
// 本文と宛先」に自分のトークンを出し、このマシンのエンジンがそれをサーバー側で持つ（ブラウザへは
// 渡さない）。送るときは、ブラウザのトークンを消費し、展開し直した本文と持っていた実行先のトークンで
// 実行先へ送る。スニペットや宛先が途中で変われば、どちらかの証拠が合わずに断られる。

// remoteCommandConfirmationLimit は、このマシンのエンジンが同時に持つ、実行先のトークンの数の
// 上限である。確認のダイアログは1つずつ開くので、ほとんどは1つで、多くても数個である。
const remoteCommandConfirmationLimit = 32

// remoteCommandConfirmation は、実行先のエンジンが出した確認のトークンと証拠である。
type remoteCommandConfirmation struct {
	evidence  string
	token     string
	expiresAt time.Time
}

// remoteCommandConfirmations は、このマシンの証拠（action evidence）ごとに、実行先のトークンを持つ。
type remoteCommandConfirmations struct {
	mutex   sync.Mutex
	entries map[string]remoteCommandConfirmation
}

func newRemoteCommandConfirmations() *remoteCommandConfirmations {
	return &remoteCommandConfirmations{entries: map[string]remoteCommandConfirmation{}}
}

func (store *remoteCommandConfirmations) keep(localEvidence string, confirmation remoteCommandConfirmation) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	now := time.Now()
	for key, entry := range store.entries {
		if !now.Before(entry.expiresAt) {
			delete(store.entries, key)
		}
	}
	if _, replacing := store.entries[localEvidence]; !replacing && len(store.entries) >= remoteCommandConfirmationLimit {
		// 一番早く切れるものを捨てる。捨てたものを確かめて送ろうとすると、確認し直しを求める。
		oldest := ""
		for key, entry := range store.entries {
			if oldest == "" || entry.expiresAt.Before(store.entries[oldest].expiresAt) {
				oldest = key
			}
		}
		delete(store.entries, oldest)
	}
	store.entries[localEvidence] = confirmation
}

// take は、localEvidence の実行先のトークンを、一度だけ返す。
func (store *remoteCommandConfirmations) take(localEvidence string) (remoteCommandConfirmation, bool) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	entry, found := store.entries[localEvidence]
	delete(store.entries, localEvidence)
	if !found || !time.Now().Before(entry.expiresAt) {
		return remoteCommandConfirmation{}, false
	}
	return entry, true
}

// remoteCommandPlan は、このマシンで展開したコマンドと、送る前に確かめ直せるこのマシンの証拠である。
type remoteCommandPlan struct {
	command snippets.PreparedCommand
	// evidence は伏せた形から、actionEvidence は展開した本文から作る。どちらも実行先へ尋ねずに
	// 作り直せるものだけを入れる。
	evidence       string
	actionEvidence string
}

func (h RemoteEngineHandlers) remoteCommandPlan(alias string, request api.TerminalCommandPreviewRequest) (remoteCommandPlan, error) {
	if h.Snippets == nil || len(request.Targets) == 0 || len(request.Targets) > snippets.MaxTargets {
		return remoteCommandPlan{}, snippets.ErrInvalidTarget
	}
	command, err := h.Snippets.PrepareTerminalCommand(snippets.CommandRequest{
		SnippetID: stringValue(request.SnippetId), Command: stringValue(request.Command), Inputs: request.Inputs,
	})
	if err != nil {
		return remoteCommandPlan{}, err
	}
	if len(command.Command) > terminal.MaxCommandBytes {
		return remoteCommandPlan{}, terminal.ErrCommandTooLarge
	}
	submit := request.Submit == nil || *request.Submit
	evidence, err := remoteCommandEvidence(alias, command.Evidence, request.Targets, submit)
	if err != nil {
		return remoteCommandPlan{}, err
	}
	actionEvidence, err := remoteCommandEvidence(alias, command.ActionEvidence, request.Targets, submit)
	if err != nil {
		return remoteCommandPlan{}, err
	}
	return remoteCommandPlan{command: command, evidence: evidence, actionEvidence: actionEvidence}, nil
}

func remoteCommandEvidence(alias, commandEvidence string, targets []api.TerminalCommandTargetRequest, submit bool) (string, error) {
	encoded, err := json.Marshal(struct {
		RunTarget       string                             `json:"runTarget"`
		CommandEvidence string                             `json:"commandEvidence"`
		Targets         []api.TerminalCommandTargetRequest `json:"targets"`
		Submit          bool                               `json:"submit"`
	}{alias, commandEvidence, targets, submit})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// combinedReviewEvidence は、ダイアログに出したもの（このマシンの伏せた形と、実行先の宛先）の証拠
// である。プレビューの中だけで比べるので、実行先の証拠を含めてよい。
func combinedReviewEvidence(localEvidence, remoteReviewEvidence string) string {
	digest := sha256.Sum256([]byte(localEvidence + "\x00" + remoteReviewEvidence))
	return hex.EncodeToString(digest[:])
}

// remoteCommandActionTarget は、ブラウザのトークンを結び付ける操作の対象である。このマシンの
// セッションへの送信（terminalCommandActionTarget）と取り違えないよう、実行先の名前を入れる。
func remoteCommandActionTarget(alias string) string { return "run-target:" + alias }

// PreviewRemoteCommand は、実行先のセッションへのコマンドの確認を用意する。
func (h RemoteEngineHandlers) PreviewRemoteCommand(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.TerminalCommandPreviewRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	plan, err := h.remoteCommandPlan(alias, request)
	if err != nil {
		return terminalCommandProblem(c, err)
	}
	issueAction := request.IssueAction == nil || *request.IssueAction
	remoteRequest := api.TerminalCommandPreviewRequest{
		Command: &plan.command.Command, Inputs: map[string]string{}, Targets: request.Targets,
		Submit: request.Submit, IssueAction: &issueAction,
	}
	var remote api.TerminalCommandPreview
	if handled, err := h.callRemoteCommandAPI(c, alias, remoteCommandCall{path: "/api/v1/terminal/commands/preview", request: remoteRequest}, &remote); handled {
		return err
	}
	reviewEvidence := combinedReviewEvidence(plan.evidence, remote.ReviewEvidence)
	if request.ExpectedReviewEvidence != nil &&
		subtle.ConstantTimeCompare([]byte(reviewEvidence), []byte(*request.ExpectedReviewEvidence)) != 1 {
		return problem(c, http.StatusConflict, "terminal_command_preview_changed")
	}
	var issued api.IssueActionResponse
	if issueAction {
		expiresAt, parseErr := time.Parse(time.RFC3339, remote.ActionExpiresAt)
		if parseErr != nil || remote.ActionToken == "" {
			return problem(c, http.StatusBadGateway, "remote_engine_unreachable")
		}
		token, allowed, response := h.Actions.issueEvidence(c, session.ActionTerminalCommand, remoteCommandActionTarget(alias), plan.actionEvidence)
		if !allowed {
			return response
		}
		h.commands.keep(plan.actionEvidence, remoteCommandConfirmation{evidence: remote.Evidence, token: remote.ActionToken, expiresAt: expiresAt})
		issued = token
	}
	// 実行先が返した宛先のコマンド（展開した本文）は、ブラウザへ渡さない。このマシンの伏せた形に替える。
	shown := plan.command.Display
	if request.RevealCommand != nil && *request.RevealCommand {
		shown = plan.command.Command
	}
	targets := make([]api.TerminalCommandPreviewTarget, 0, len(remote.Targets))
	for _, target := range remote.Targets {
		target.Command = shown
		targets = append(targets, target)
	}
	return c.JSON(http.StatusOK, api.TerminalCommandPreview{
		SnippetId: plan.command.SnippetID, Evidence: plan.evidence, ReviewEvidence: reviewEvidence,
		ActionToken: issued.Token, ActionExpiresAt: issued.ExpiresAt, Targets: targets,
	})
}

// DispatchRemoteCommand は、確かめたコマンドを実行先のセッションへ送る。
func (h RemoteEngineHandlers) DispatchRemoteCommand(c *echo.Context) error {
	alias, ok := aliasParam(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.TerminalCommandDispatchRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(request.Evidence) != sha256.Size*2 {
		return problem(c, http.StatusBadRequest, "invalid_terminal_command")
	}
	plan, err := h.remoteCommandPlan(alias, terminalCommandPreviewFromDispatch(request))
	if err != nil || subtle.ConstantTimeCompare([]byte(plan.evidence), []byte(request.Evidence)) != 1 {
		return problem(c, http.StatusConflict, "terminal_command_preview_changed")
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionTerminalCommand, remoteCommandActionTarget(alias), plan.actionEvidence); !allowed {
		return response
	}
	confirmation, found := h.commands.take(plan.actionEvidence)
	if !found {
		return problem(c, http.StatusConflict, "terminal_command_preview_changed")
	}
	remoteRequest := api.TerminalCommandDispatchRequest{
		Command: &plan.command.Command, Inputs: map[string]string{}, Targets: request.Targets,
		Submit: request.Submit, Evidence: confirmation.evidence,
	}
	var results api.TerminalCommandDispatchResponse
	call := remoteCommandCall{path: "/api/v1/terminal/commands", request: remoteRequest, actionToken: confirmation.token}
	if handled, err := h.callRemoteCommandAPI(c, alias, call, &results); handled {
		return err
	}
	return c.JSON(http.StatusOK, results)
}

// remoteCommandCall は、実行先のエンジンのコマンドの受け口への要求1つである。
type remoteCommandCall struct {
	path        string
	request     any
	actionToken string
}

var errRemoteAnswerUnreadable = errors.New("the remote engine's command answer is unreadable")

// callRemoteCommandAPI は、call を実行先へ送り、成功の答えを answer へ読む。答えを書き終えた
// （断りを返した）ときは handled を返す。実行先の断り（宛先が無い、証拠が合わない、など）は、
// その状態コードと本文のまま返す。どれも画面が実行先のことと読める語である。
func (h RemoteEngineHandlers) callRemoteCommandAPI(c *echo.Context, alias string, call remoteCommandCall, answer any) (handled bool, _ error) {
	body, err := json.Marshal(call.request)
	if err != nil {
		return true, unexpectedProblem(c, "terminal_command_failed", err)
	}
	reply, err := h.Engines.CallTerminalAPI(c.Request().Context(), alias, remoteengine.TerminalCall{
		Method: http.MethodPost, Path: call.path, ContentType: echo.MIMEApplicationJSON, Body: body, ActionToken: call.actionToken,
	})
	if err != nil {
		return true, remoteEngineProblem(c, err)
	}
	if reply.Status != http.StatusOK {
		return true, c.Blob(reply.Status, "application/problem+json", reply.Body)
	}
	decoder := json.NewDecoder(bytes.NewReader(reply.Body))
	if err := decoder.Decode(answer); err != nil {
		return true, unexpectedProblem(c, "terminal_command_failed", errors.Join(errRemoteAnswerUnreadable, err))
	}
	return false, nil
}
