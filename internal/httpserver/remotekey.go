package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/diagnostics"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/remotekey"
	"sshc/internal/session"
	"sshc/internal/validate"
)

type preparedRemoteKeyPlan struct {
	key    remotekey.PublicKey
	plan   remotekey.Plan
	report effective.Report
	config []byte
}

// RemoteKeyHandlers はリモートホストに公開鍵を登録する。
//
// Diagnostics は、確認画面が表示する接続先と、接続時に実行される
// 実行可能ディレクティブを供給する。登録直前にも同じ計画を再構築し、
// 表示後に設定が変わっていれば接続前に拒否する。
type RemoteKeyHandlers struct {
	Service     *remotekey.Service
	Diagnostics *diagnostics.Service
	Actions     ActionHandlers
}

// prepare は確認時と実行時に同じ計画を組み立てる。ここで得た全項目の
// ダイジェストが一致しなければ、リモート接続は開始されない。
func (h RemoteKeyHandlers) prepare(alias, keyPath, publicKey string) (preparedRemoteKeyPlan, error) {
	if err := validate.Alias(alias); err != nil {
		return preparedRemoteKeyPlan{}, err
	}
	key, fingerprint, err := remotekey.ParsePublicKey(publicKey)
	if err != nil {
		return preparedRemoteKeyPlan{}, err
	}
	key.Path = keyPath
	snapshot, err := h.Diagnostics.ConnectionSnapshot(alias)
	if err != nil {
		return preparedRemoteKeyPlan{}, err
	}
	return preparedRemoteKeyPlan{
		key:    key,
		plan:   h.Service.Plan(alias, key, fingerprint, snapshot.User, snapshot.Hostname, snapshot.Port, "engine"),
		report: snapshot.Report,
		config: snapshot.Config,
	}, nil
}

func remoteKeyPlanProblem(c *echo.Context, err error) error {
	if errors.Is(err, remotekey.ErrInvalidPublicKey) || errors.Is(err, validate.ErrUnsafeAlias) {
		return remoteKeyProblem(c, err)
	}
	return problem(c, http.StatusInternalServerError, "config_unreadable")
}

func registerRemoteKeyRoutes(engine *echo.Echo, handlers RemoteKeyHandlers) {
	engine.POST("/api/v1/remote-keys/plan", handlers.Plan)
	engine.POST("/api/v1/remote-keys/register", handlers.Register)
}

func remoteKeyProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, remotekey.ErrInvalidPublicKey):
		return problem(c, http.StatusBadRequest, "invalid_public_key")
	case errors.Is(err, remotekey.ErrNotAcknowledged):
		return problem(c, http.StatusConflict, "executable_directive_not_acknowledged")
	case errors.Is(err, remotekey.ErrUnsupportedRemote):
		return problem(c, http.StatusUnprocessableEntity, "unsupported_remote")
	case errors.Is(err, validate.ErrUnsafeAlias):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	return problem(c, http.StatusInternalServerError, "registration_failed")
}

// Plan はリモートホストに接続せずに変更内容を説明する。
func (h RemoteKeyHandlers) Plan(c *echo.Context) error {
	var request api.RemoteKeyPlanRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	// 接続先はこの application 自身が設定を読んだ結果によるもので、
	// プロセスを必要としない。plan の裏で ssh -G が実行されることはない。
	prepared, err := h.prepare(request.Alias, request.KeyPath, request.PublicKey)
	if err != nil {
		return remoteKeyPlanProblem(c, err)
	}
	plan := prepared.plan
	issued, err := h.Actions.issueEvidence(c, session.ActionRemoteKeyRegister, plan.Alias,
		plan.Evidence(prepared.report.Evidence(), prepared.config))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, api.RemoteKeyPlan{
		Alias:                plan.Alias,
		User:                 plan.User,
		Hostname:             plan.Hostname,
		Port:                 plan.Port,
		ValuesFrom:           plan.ValuesFrom,
		Fingerprint:          plan.Fingerprint,
		KeyPath:              plan.KeyPath,
		KeyLine:              plan.KeyLine,
		RemotePath:           plan.RemotePath,
		Routine:              plan.Routine,
		Supported:            plan.Supported,
		Manual:               plan.Manual,
		ExecutableDirectives: describeDirectives(prepared.report.Directives),
		ActionToken:          issued.Token,
		ActionExpiresAt:      issued.ExpiresAt,
	})
}

// Register は確認が消費された後に鍵をインストールする。
func (h RemoteKeyHandlers) Register(c *echo.Context) error {
	var request api.RemoteKeyRegisterRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	prepared, err := h.prepare(request.Alias, request.KeyPath, request.PublicKey)
	if err != nil {
		return remoteKeyPlanProblem(c, err)
	}

	if allowed, response := h.Actions.consumeEvidence(c, session.ActionRemoteKeyRegister, request.Alias,
		prepared.plan.Evidence(prepared.report.Evidence(), prepared.config)); !allowed {
		return response
	}

	result, err := h.Service.Register(c.Request().Context(), prepared.report, prepared.config,
		request.Alias, prepared.key, request.AcknowledgeExecutable)
	if err != nil {
		return remoteKeyProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.RemoteKeyRegisterResponse{
		Outcome:  result.Outcome,
		ExitCode: result.ExitCode,
		// ssh は読み込んだファイルを絶対パスで指定するため、アカウント名は
		// 出力がこのプロセスを出る前に取り除かれる。
		Stderr:    platform.SanitiseHomePaths(result.Stderr, h.Diagnostics.Home()),
		Truncated: result.Truncated,
	})
}
