package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/connectionattempt"
	"sshc/internal/sshclient"
)

// SFTP のタブが最初の一覧を取る要求は、SSH の接続が終わるまで返らない。その要求が
// attempt を名乗っていれば、接続の段階と接続ログを預ける。Web UI はそれを別の要求で読み、
// ターミナルと同じく接続のあいだの経過を表示する。known_hosts に無いホスト鍵は、ターミナルと
// 同じくタブの中で尋ね、別の要求で答えを受け取る。attempt を名乗らない要求（転送など）は、
// 尋ねる相手がいないので未知の鍵を断る。

// observeConnection は、要求が attempt を名乗っていれば、その接続の経過を預ける ctx と、
// 要求の処理が終わったときに結果を渡す関数を返す。
func (h SFTPHandlers) observeConnection(c *echo.Context) (context.Context, func(error), error) {
	ctx := c.Request().Context()
	id := c.QueryParam("attempt")
	if id == "" {
		return ctx, func(error) {}, nil
	}
	attempt, err := h.Attempts.Begin(c.Param("alias"), id)
	if err != nil {
		return nil, nil, err
	}
	observer := sshclient.ConnectionObserver{Progress: attempt.Progress, Log: attempt, ConfirmHostKey: attempt.ConfirmHostKey}
	return sshclient.WithConnectionObserver(ctx, observer), attempt.Finish, nil
}

// ConnectionAttempt は、一覧の要求が預けた接続の段階と接続ログを返す。
func (h SFTPHandlers) ConnectionAttempt(c *echo.Context) error {
	return h.describeAttempt(c)
}

// AnswerHostKey は、一覧の要求の接続が待っている、known_hosts に無いホスト鍵の問いに、
// 画面のボタンから答える。その鍵についての問いをもう待っていなければ 409 を返し、何もしない。
func (h SFTPHandlers) AnswerHostKey(c *echo.Context) error {
	answer, ok := decodeHostKeyAnswer(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	err := h.Attempts.AnswerHostKey(c.Param("alias"), c.Param("attempt"), answer.Fingerprint, answer.Accept)
	switch {
	case errors.Is(err, connectionattempt.ErrNotFound):
		return problem(c, http.StatusNotFound, "sftp_connection_attempt_not_found")
	case errors.Is(err, connectionattempt.ErrNoHostKeyQuestion):
		return problem(c, http.StatusConflict, "sftp_no_host_key_question")
	case err != nil:
		return unexpectedProblem(c, "sftp_host_key_answer_failed", err)
	}
	return h.describeAttempt(c)
}

// describeAttempt は、経路の alias と attempt が指す試行の今の様子を返す。
func (h SFTPHandlers) describeAttempt(c *echo.Context) error {
	snapshot, ok := h.Attempts.Snapshot(c.Param("alias"), c.Param("attempt"))
	if !ok {
		return problem(c, http.StatusNotFound, "sftp_connection_attempt_not_found")
	}
	described := api.SFTPConnectionAttempt{Log: snapshot.Log, Finished: snapshot.Finished}
	if described.Log == nil {
		described.Log = []string{}
	}
	if snapshot.Progress != nil {
		described.Progress = describeConnectionProgress(*snapshot.Progress)
	}
	if snapshot.HostKeyQuestion != nil {
		described.HostKeyQuestion = describeHostKeyQuestion(*snapshot.HostKeyQuestion)
	}
	return c.JSON(http.StatusOK, described)
}
