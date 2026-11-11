package httpserver

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
)

// 同期の設定の書き出しと取り込み。設定済みのマシンが同期先と認証情報を同期鍵で封じた
// 文書を渡し、別のマシンは同期鍵を入力してそれを開き、同じ同期先に参加する。

// ExportSetup は、このマシンの同期先と認証情報を、Vault にある同期鍵で封じて返す。
//
// 認証情報がengineの外へ出るのは、この封じた形だけである。同期鍵を知らないと開けない。
func (h SyncHandlers) ExportSetup(c *echo.Context) error {
	if h.Vault == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	settings, err := h.Vault.SyncSettings()
	if err != nil {
		if vaultUnavailable(err) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		return unexpectedProblem(c, "vault_unreadable", err)
	}
	if settings.Endpoint == "" || settings.Bucket == "" || settings.AccessKeyID == "" {
		return syncProblem(c, remotesync.ErrNotConfigured)
	}
	if settings.Key == "" {
		return problem(c, http.StatusConflict, "sync_key_missing")
	}
	document, err := remotesync.SealSettingsExport(remotesync.SettingsExport{
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path, Region: settings.Region,
		Credentials: remotesync.Credentials{
			AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
		},
	}, settings.Key)
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncSetupExport{Document: document, KeyGenerated: remotesync.KeyLooksGenerated(settings.Key)}
	if rows, fits := remotesync.SettingsExportQRCode(document); fits {
		response.QrRows = &rows
	}
	return c.JSON(http.StatusOK, response)
}

// ImportSetup は、書き出した文書を入力された鍵で開き、その同期先を確かめてから、
// 設定と鍵をまとめて保存する。文書を開いた鍵が、そのままスナップショットを確かめる鍵になる。
func (h SyncHandlers) ImportSetup(c *echo.Context) error {
	var request api.SyncSetupImportRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	direction, ok := remotesync.ParseDirection(string(request.Direction))
	if !ok {
		return problem(c, http.StatusBadRequest, "unknown_sync_direction")
	}
	syncKey := strings.TrimSpace(request.Key)
	if syncKey == "" || len(syncKey) > remotesync.MaxKeyLength {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	// 保存できないと分かっているなら、鍵導出と同期先への問い合わせを始めない。
	if h.Vault == nil || !h.Vault.Unlocked() {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	exported, err := remotesync.OpenSettingsExport(request.Document, syncKey)
	if err != nil {
		return syncProblem(c, err)
	}
	// 文書は別のマシンから届いた入力である。手で打った設定と同じ検証に通す。
	config, credentials, err := syncSetupInput(api.SyncSetupCheckRequest{
		Endpoint: exported.Endpoint, Bucket: exported.Bucket, Path: &exported.Path, Region: &exported.Region,
	}, exported.Credentials)
	if err != nil {
		return syncProblem(c, remotesync.ErrSettingsExportRejected)
	}
	config.Direction = direction
	client := h.objectStoreClient(config, credentials)
	if err := h.Service.JoinSetup(c.Request().Context(), config, credentials, client, syncKey,
		h.persistSyncSetup(config, credentials, syncKey)); err != nil {
		return setupFailureProblem(c, err)
	}
	return c.JSON(http.StatusOK, h.savedSetupResponse())
}
