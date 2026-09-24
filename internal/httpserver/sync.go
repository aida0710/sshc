package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/session"
)

var (
	errSyncKeyMissing          = errors.New("synchronization key is missing")
	errSyncSetupInvalidRequest = errors.New("invalid sync setup request")
)

// SyncHandlers はリモートのスナップショットを提供する。
type SyncHandlers struct {
	Service *remotesync.Service
	// Secrets は、同期対象外の暗号化ファイルに object store 設定を保存する。
	// nil の場合は設定を保存できないので、/sync/setup は vault_locked で断る。
	Secrets *secret.Service
	// ObjectStoreHTTP は bucket へ話す HTTP client である。nil なら既定の client を
	// 使う。このパッケージのテストはネットワークに触れてはならないので、bucket の
	// 代わりへ要求を渡す client をここへ注入する。
	ObjectStoreHTTP *http.Client
	// Auto は自動同期の巡回処理。nil の場合は無効として応答する。
	Auto *remotesync.Auto
	// Actions binds a destructive force push to the exact binding generation,
	// target identity, and remote ETag which the user inspected and confirmed.
	Actions ActionHandlers
}

func (h SyncHandlers) objectStoreClient(config remotesync.Config, credentials remotesync.Credentials) *remotesync.Client {
	client := remotesync.NewClient(config, credentials)
	client.HTTP = h.ObjectStoreHTTP
	return client
}

func registerSyncRoutes(engine *echo.Echo, handlers SyncHandlers) {
	engine.GET("/api/v1/sync", handlers.Status)
	engine.POST("/api/v1/sync/setup/check", handlers.CheckSetup)
	engine.PUT("/api/v1/sync/setup", handlers.CompleteSetup)
	engine.GET("/api/v1/sync/exclusions", handlers.Exclusions)
	engine.PUT("/api/v1/sync/exclusions", handlers.SaveExclusions)
	engine.PUT("/api/v1/sync/key", handlers.SetKey)
	engine.PUT("/api/v1/sync/auto", handlers.SetAuto)
	engine.POST("/api/v1/sync/now", handlers.Now)
	engine.GET("/api/v1/sync/push", handlers.PushDraft)
	engine.POST("/api/v1/sync/push", handlers.Push)
	engine.POST("/api/v1/sync/force-push", handlers.ForcePush)
	engine.POST("/api/v1/sync/pull", handlers.Pull)
	engine.GET("/api/v1/sync/bucket", handlers.Bucket)
	engine.GET("/api/v1/sync/history", handlers.History)
	engine.POST("/api/v1/sync/history/diff", handlers.HistoryDiff)
}

func addSyncActions(registry actionRegistry, service *remotesync.Service) {
	registry[session.ActionSyncForcePush] = actionKind{
		evidence: func(parent context.Context, target string) (string, error) {
			ctx, cancel := context.WithTimeout(parent, 30*time.Second)
			defer cancel()
			confirmation, err := service.ForcePushConfirmation(ctx, target)
			return confirmation.Evidence, err
		},
		fail: syncProblem,
	}
}

// restore は、vault のロック解除後に保存済み設定から client を構成する。
// ロック中はエラーにせず、status の Locked フィールドで通知する。
func (h SyncHandlers) restore() {
	if h.Secrets == nil || !h.Secrets.Unlocked() || h.Service.Configured() {
		return
	}
	settings, err := h.Secrets.SyncSettings()
	if err != nil || settings.Bucket == "" {
		return
	}
	direction, ok := remotesync.ParseDirection(settings.Direction)
	if !ok {
		return
	}
	credentials := remotesync.Credentials{
		AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
	}
	config := remotesync.Config{
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path,
		Region: settings.Region, Direction: direction,
	}
	_, _ = h.Service.ConfigureIfUnconfigured(config, credentials, h.objectStoreClient(config, credentials))
}

func snapshotSummaryResponse(summary remotesync.SnapshotSummary) api.SnapshotSummary {
	return api.SnapshotSummary{
		CreatedAt: summary.CreatedAt, FileCount: summary.FileCount,
		SourceBytes: summary.SourceBytes, SnapshotBytes: summary.SnapshotBytes,
	}
}

func syncOperationResponse(operation remotesync.SyncOperation) api.SyncOperation {
	response := api.SyncOperation{
		Kind: api.SyncOperationKind(operation.Kind), Summary: snapshotSummaryResponse(operation.Summary),
		CompletedAt: operation.CompletedAt,
	}
	switch operation.Kind {
	case remotesync.OperationPush:
		response.ObjectCount = &operation.ObjectCount
		response.UploadedBytes = &operation.UploadedBytes
	case remotesync.OperationApply:
		response.DownloadedBytes = &operation.DownloadedBytes
		response.Written = &operation.Written
		response.Removed = &operation.Removed
	}
	return response
}

func (h SyncHandlers) statusResponse() api.SyncStatus {
	h.restore()
	endpoint, bucket, path, region := h.Service.Target()
	state := h.Service.SyncState()
	response := api.SyncStatus{
		Configured: h.Service.Configured(),
		Endpoint:   endpoint,
		Bucket:     bucket,
		Path:       &path,
		Region:     &region,
		Synced:     state.Synced,
		Direction:  api.SyncDirection(h.Service.Direction()),
		// access key と secret は返さず、入力欄が空になる理由を Locked で示す。
		Locked:        h.Secrets != nil && !h.Secrets.Unlocked(),
		KeyConfigured: h.keyConfigured(),
		Auto:          h.autoResponse(),
	}
	if suffix := h.Service.AccessKeySuffix(); suffix != "" {
		response.AccessKeySuffix = &suffix
	}
	if state.Synced {
		response.LastSyncedAt = &state.At
		response.Origin = &state.Origin
		response.FileCount = &state.Files
	}
	if state.LastOperation != nil {
		operation := syncOperationResponse(*state.LastOperation)
		response.LastOperation = &operation
	}
	return response
}

func (h SyncHandlers) status(c *echo.Context) error {
	response := h.statusResponse()
	return c.JSON(http.StatusOK, response)
}

func (h SyncHandlers) Status(c *echo.Context) error { return h.status(c) }

func syncStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
