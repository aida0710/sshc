package httpserver

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/sshclient"
)

// SFTP のタブが最初の一覧を取る要求は、SSH の接続が終わるまで返らない。その要求が
// attempt を名乗っていれば、接続の段階と接続ログを預ける。Web UI はそれを別の要求で読み、
// ターミナルと同じく接続のあいだの経過を表示する。

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
	observer := sshclient.ConnectionObserver{Progress: attempt.Progress, Log: attempt}
	return sshclient.WithConnectionObserver(ctx, observer), attempt.Finish, nil
}

// ConnectionAttempt は、一覧の要求が預けた接続の段階と接続ログを返す。
func (h SFTPHandlers) ConnectionAttempt(c *echo.Context) error {
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
	return c.JSON(http.StatusOK, described)
}
