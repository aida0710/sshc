package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/session"
)

var errSyncKeyMissing = errors.New("synchronization key is missing")

// SyncHandlers はリモートのスナップショットを提供する。
type SyncHandlers struct {
	Service *remotesync.Service
	// Secrets は、同期対象外の暗号化ファイルに object store 設定を保存する。
	// nil の場合、設定はプロセス内だけで有効になる。
	Secrets *secret.Service
	// Reach は設定が保存される前にそれが機能するかを尋ねる。これが
	// 注入されているのは、設定を保存するのはこのハンドラの仕事だが
	// bucket に到達することはそうではないからで、またこのパッケージの
	// どのテストもネットワークに触れてはならないからだ。nil の場合は
	// 実際のチェックを意味する。配線し忘れが暗黙に「問題なし」になってはならない。
	Reach func(ctx context.Context, client *remotesync.Client, key string) error
	// Auto は自動同期の巡回処理。nil の場合は無効として応答する。
	Auto *remotesync.Auto
	// Actions binds a destructive force push to the exact remote ETag which the
	// user inspected and confirmed.
	Actions ActionHandlers
}

func (h SyncHandlers) reach(ctx context.Context, client *remotesync.Client, key string) error {
	if h.Reach == nil {
		return remotesync.Check(ctx, client, key)
	}
	return h.Reach(ctx, client, key)
}

func registerSyncRoutes(engine *echo.Echo, handlers SyncHandlers) {
	engine.GET("/api/v1/sync", handlers.Status)
	engine.POST("/api/v1/sync/setup/check", handlers.CheckSetup)
	engine.PUT("/api/v1/sync/setup", handlers.CompleteSetup)
	engine.PUT("/api/v1/sync/settings", handlers.Configure)
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

func syncSetupInput(request api.SyncSetupCheckRequest) (remotesync.Config, remotesync.Credentials, error) {
	if len(request.Endpoint) == 0 || len(request.Endpoint) > 2048 ||
		len(request.AccessKeyId) == 0 || len(request.AccessKeyId) > 512 ||
		len(request.SecretAccessKey) == 0 || len(request.SecretAccessKey) > 512 ||
		(request.Path != nil && len(*request.Path) > 255) ||
		(request.Region != nil && len(*request.Region) > 64) {
		return remotesync.Config{}, remotesync.Credentials{}, errors.New("invalid_request")
	}
	parsed, err := url.Parse(request.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return remotesync.Config{}, remotesync.Credentials{}, errors.New("endpoint_must_be_https")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return remotesync.Config{}, remotesync.Credentials{}, errors.New("endpoint_must_have_no_path")
	}
	if !safeBucketName(request.Bucket) {
		return remotesync.Config{}, remotesync.Credentials{}, errors.New("unsafe_bucket_name")
	}
	path := ""
	if request.Path != nil {
		path = strings.Trim(*request.Path, "/")
	}
	if !safeObjectPath(path) {
		return remotesync.Config{}, remotesync.Credentials{}, errors.New("unsafe_object_path")
	}
	region := "auto"
	if request.Region != nil && *request.Region != "" {
		region = *request.Region
	}
	return remotesync.Config{
		Endpoint: strings.TrimRight(request.Endpoint, "/"), Bucket: request.Bucket,
		Path: path, Region: region, Direction: remotesync.DirectionBoth,
	}, remotesync.Credentials{AccessKeyID: request.AccessKeyId, SecretAccessKey: request.SecretAccessKey}, nil
}

func setupInputProblem(c *echo.Context, err error) error {
	switch err.Error() {
	case "endpoint_must_be_https", "endpoint_must_have_no_path", "unsafe_bucket_name", "unsafe_object_path":
		return problem(c, http.StatusBadRequest, err.Error())
	default:
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
}

// CheckSetup probes an exact destination without persisting credentials.
func (h SyncHandlers) CheckSetup(c *echo.Context) error {
	var request api.SyncSetupCheckRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	config, credentials, err := syncSetupInput(request)
	if err != nil {
		return setupInputProblem(c, err)
	}
	inspection, err := remotesync.InspectSetupTarget(c.Request().Context(), remotesync.NewClient(config, credentials), config)
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncSetupCheckResponse{
		State: api.SyncSetupTargetState(inspection.State), HistoryPresent: inspection.HistoryPresent,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if inspection.ETag != "" {
		response.Etag = &inspection.ETag
	}
	return c.JSON(http.StatusOK, response)
}

// CompleteSetup verifies an existing snapshot key and persists all secret
// settings only after every network and cryptographic check has succeeded.
func (h SyncHandlers) CompleteSetup(c *echo.Context) error {
	var request api.SyncSetupRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	config, credentials, err := syncSetupInput(api.SyncSetupCheckRequest{
		Endpoint: request.Endpoint, Bucket: request.Bucket, Path: request.Path, Region: request.Region,
		AccessKeyId: request.AccessKeyId, SecretAccessKey: request.SecretAccessKey,
	})
	if err != nil {
		return setupInputProblem(c, err)
	}
	direction, ok := remotesync.ParseDirection(string(request.Direction))
	if !ok {
		return problem(c, http.StatusBadRequest, "unknown_sync_direction")
	}
	if !request.ExpectedState.Valid() ||
		(request.ExpectedState == api.Existing && request.ExpectedETag == nil) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	config.Direction = direction
	if h.Secrets == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	key := strings.TrimSpace(request.Key)
	generated := false
	if key == "" {
		if request.ExpectedState != api.Empty {
			return problem(c, http.StatusBadRequest, "sync_key_missing")
		}
		key, err = remotesync.NewKey()
		if err != nil {
			return problem(c, http.StatusInternalServerError, "key_generation_failed")
		}
		generated = true
	}
	if len(key) > 1024 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	expected := remotesync.SetupInspection{
		State: remotesync.SetupTargetState(request.ExpectedState), HistoryPresent: request.HistoryPresent,
	}
	if request.ExpectedETag != nil {
		expected.ETag = *request.ExpectedETag
	}
	client := remotesync.NewClient(config, credentials)
	err = h.Service.CompleteSetup(c.Request().Context(), config, credentials, client, expected, key, func() error {
		return h.Secrets.SetSyncSettings(secret.SyncSettings{
			Endpoint: config.Endpoint, Bucket: config.Bucket, Path: config.Path, Region: config.Region,
			AccessKeyID: credentials.AccessKeyID, SecretAccessKey: credentials.SecretAccessKey,
			Direction: string(direction), Key: key,
		})
	})
	if err != nil {
		if errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		return syncProblem(c, err)
	}
	if h.Auto != nil {
		h.Auto.ResetRemoteCache()
	}
	response := api.SyncSetupResponse{Status: h.statusResponse()}
	if generated {
		response.GeneratedKey = &key
	}
	return c.JSON(http.StatusOK, response)
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
	_, _ = h.Service.ConfigureIfUnconfigured(config, credentials, remotesync.NewClient(config, credentials))
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

// Configure はこのマシンをある bucket に向ける。
//
// credentials はマスターパスワードで暗号化し、同期対象外の専用ファイルへ保存する。
// 同期先の資格情報を同期スナップショット自体へ含めない。
func (h SyncHandlers) Configure(c *echo.Context) error {
	var request api.SyncSettingsRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	config, credentials, err := syncSetupInput(api.SyncSetupCheckRequest{
		Endpoint: request.Endpoint, Bucket: request.Bucket, Path: request.Path, Region: request.Region,
		AccessKeyId: request.AccessKeyId, SecretAccessKey: request.SecretAccessKey,
	})
	if err != nil {
		return setupInputProblem(c, err)
	}
	direction, ok := remotesync.ParseDirection(string(request.Direction))
	if !ok {
		return problem(c, http.StatusBadRequest, "unknown_sync_direction")
	}
	config.Direction = direction
	// 保存する前に試す。一度も試されなかった設定は、typo が最初の
	// push で何時間も後に別の場所で表面化する設定になってしまう。
	// ここは、ユーザーが自分の打ったものをまだ見られる唯一の画面である。
	client := remotesync.NewClient(config, credentials)
	if err := h.reach(c.Request().Context(), client, remotesync.ObjectKeyFor(config)); err != nil {
		return syncProblem(c, err)
	}

	// 使われる前に保存する。これにより、次の実行では消えているはずの
	// 設定を使ったと応答が主張してしまうことはない。
	persist := func() error { return nil }
	if h.Secrets != nil {
		persist = func() error {
			return h.Secrets.SetSyncSettings(secret.SyncSettings{
				Endpoint: config.Endpoint, Bucket: config.Bucket, Path: config.Path, Region: config.Region,
				AccessKeyID: credentials.AccessKeyID, SecretAccessKey: credentials.SecretAccessKey,
				Direction: string(direction),
			})
		}
	}
	if err := h.Service.Reconfigure(config, credentials, client, persist); err != nil {
		if errors.Is(err, remotesync.ErrRecoveryTargetChange) || errors.Is(err, remotesync.ErrRecoveryRequired) {
			return syncProblem(c, err)
		}
		if h.Secrets != nil {
			if errors.Is(err, secret.ErrLocked) {
				return problem(c, http.StatusConflict, "vault_locked")
			}
			return problem(c, http.StatusInternalServerError, "vault_failed")
		}
		return syncProblem(c, err)
	}
	if h.Auto != nil {
		h.Auto.ResetRemoteCache()
	}
	return h.status(c)
}

// sealingKey は、vault に保存した同期専用の暗号鍵を返す。
// 取得できない場合は HTTP 応答を書き込み、ok=false を返す。
func (h SyncHandlers) sealingKey(c *echo.Context) (string, bool, error) {
	key, err := h.currentSyncKey()
	switch {
	case errors.Is(err, secret.ErrLocked), errors.Is(err, secret.ErrNoVault):
		return "", false, problem(c, http.StatusConflict, "vault_locked")
	case errors.Is(err, errSyncKeyMissing):
		return "", false, problem(c, http.StatusConflict, "sync_key_missing")
	case err != nil:
		return "", false, problem(c, http.StatusInternalServerError, "vault_unreadable")
	}
	return key, true, nil
}

func (h SyncHandlers) currentSyncKey() (string, error) {
	if h.Secrets == nil {
		return "", errSyncKeyMissing
	}
	settings, err := h.Secrets.SyncSettings()
	if err != nil {
		return "", err
	}
	if settings.Key == "" {
		return "", errSyncKeyMissing
	}
	return settings.Key, nil
}

func (h SyncHandlers) keyProvider() remotesync.KeyProvider {
	return h.currentSyncKey
}

func syncKeyProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, errSyncKeyMissing):
		return problem(c, http.StatusConflict, "sync_key_missing")
	case errors.Is(err, secret.ErrLocked), errors.Is(err, secret.ErrNoVault):
		return problem(c, http.StatusConflict, "vault_locked")
	default:
		return syncProblem(c, err)
	}
}

// SetKey は、このワークスペースが同期に使う鍵を決める。
//
// 本文が空なら作る。既定が生成であることに理由がある、この値は端末をまたいで
// 共有されるので、どこかに書き留められる。書き留めるなら、覚えられる必要はない。
//
// 応答は、採った鍵そのものである。平文でこれが出る唯一の場所であり、画面はこれを
// 一度だけ見せる。
func (h SyncHandlers) SetKey(c *echo.Context) error {
	h.restore()
	var request api.SyncKeyRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if h.Secrets == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	key := ""
	if request.Key != nil {
		if len(*request.Key) == 0 || len(*request.Key) > 1024 {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		key = strings.TrimSpace(*request.Key)
	}
	if key == "" {
		generated, err := remotesync.NewKey()
		if err != nil {
			return problem(c, http.StatusInternalServerError, "key_generation_failed")
		}
		key = generated
	}
	// 弱い鍵は、暗号化する段になって初めて分かるのでは遅い。ここで断る。
	if err := remotesync.ValidateKey(key); err != nil {
		if errors.Is(err, remotesync.ErrWeakPassphrase) {
			return problem(c, http.StatusBadRequest, "passphrase_too_short")
		}
		return problem(c, http.StatusInternalServerError, "vault_unreadable")
	}
	confirmHistoryLoss := request.ConfirmHistoryLoss != nil && *request.ConfirmHistoryLoss
	if err := h.Service.ReplaceKeyUsing(c.Request().Context(), key, confirmHistoryLoss, func() (string, func() error, error) {
		settings, err := h.Secrets.SyncSettings()
		if err != nil {
			return "", nil, err
		}
		commit := func() error {
			if err := h.Secrets.SetSyncKeyIfSettingsMatch(settings, key); errors.Is(err, secret.ErrSyncSettingsChanged) {
				return remotesync.ErrRemoteMoved
			} else {
				return err
			}
		}
		return settings.Key, commit, nil
	}); err != nil {
		if errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		if errors.Is(err, remotesync.ErrRemoteMoved) || errors.Is(err, remotesync.ErrWrongPassphrase) ||
			errors.Is(err, remotesync.ErrUnsupportedEnvelopeVersion) || errors.Is(err, remotesync.ErrUnsupportedVersion) ||
			errors.Is(err, remotesync.ErrNotASnapshot) || errors.Is(err, remotesync.ErrRecoveryRequired) ||
			errors.Is(err, remotesync.ErrHistoryKeyLossConfirmation) {
			return syncProblem(c, err)
		}
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
	return c.JSON(http.StatusOK, api.SyncKeyResponse{Key: key})
}

func (h SyncHandlers) Push(c *echo.Context) error {
	h.restore()
	var request api.SyncPushRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	message, err := remotesync.NormalizeCommitMessage(request.Message)
	if err != nil {
		return syncProblem(c, err)
	}
	result, err := h.Service.PushUsing(c.Request().Context(), h.keyProvider(), message)
	if err != nil {
		return syncKeyProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.PushResponse{
		Status: h.statusResponse(),
		Result: api.PushResult{
			Summary: snapshotSummaryResponse(result.Summary), ObjectCount: result.ObjectCount,
			UploadedBytes: result.UploadedBytes, CompletedAt: result.CompletedAt,
		},
	})
}

func (h SyncHandlers) PushDraft(c *echo.Context) error {
	h.restore()
	draft, err := h.Service.PushDraft()
	if err != nil {
		return syncProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SyncPushDraft{
		Message: draft.Message, Added: draft.Added, Modified: draft.Modified, Removed: draft.Removed,
	})
}

func (h SyncHandlers) ForcePush(c *echo.Context) error {
	h.restore()
	var request api.SyncPushRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	message, err := remotesync.NormalizeCommitMessage(request.Message)
	if err != nil {
		return syncProblem(c, err)
	}
	confirmation, err := h.Service.ForcePushConfirmation(c.Request().Context(), remotesync.ForcePushTarget)
	if err != nil {
		return syncProblem(c, err)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSyncForcePush,
		remotesync.ForcePushTarget, confirmation.Evidence); !allowed {
		return response
	}
	result, err := h.Service.ForcePushUsing(c.Request().Context(), h.keyProvider(), confirmation.ETag, message)
	if err != nil {
		return syncKeyProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.PushResponse{
		Status: h.statusResponse(),
		Result: api.PushResult{
			Summary: snapshotSummaryResponse(result.Summary), ObjectCount: result.ObjectCount,
			UploadedBytes: result.UploadedBytes, CompletedAt: result.CompletedAt,
		},
	})
}

func (h SyncHandlers) Bucket(c *echo.Context) error {
	h.restore()
	view, err := h.Service.BucketStatus(c.Request().Context())
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncBucketStatus{
		CheckedAt: view.CheckedAt, History: make([]api.SyncBucketObject, 0, len(view.History)),
		HistoryTruncated: view.HistoryTruncated, LocalIsLive: view.LocalIsLive,
	}
	if view.Live != nil {
		response.Live = &api.SyncBucketObject{
			Key: view.Live.Key, Size: view.Live.Size, LastModified: syncStringPointer(view.Live.LastModified),
		}
	}
	for _, item := range view.History {
		response.History = append(response.History, api.SyncBucketObject{
			Key: item.Key, Size: item.Size, LastModified: syncStringPointer(item.LastModified),
		})
	}
	return c.JSON(http.StatusOK, response)
}

func (h SyncHandlers) History(c *echo.Context) error {
	h.restore()
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	view, err := h.Service.History(c.Request().Context(), key)
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncHistory{
		CheckedAt: view.CheckedAt, HeadRevision: view.HeadRevision,
		HistoryTruncated: view.HistoryTruncated, DownloadTruncated: view.DownloadTruncated,
		DownloadedBytes: view.DownloadedBytes,
		Skipped:         view.Skipped,
		Revisions:       make([]api.SyncHistoryRevision, 0, len(view.Revisions)),
	}
	for _, revision := range view.Revisions {
		response.Revisions = append(response.Revisions, api.SyncHistoryRevision{
			Key: revision.Key, Revision: revision.Revision,
			ParentRevision: syncStringPointer(revision.ParentRevision),
			Message:        revision.Message,
			CreatedAt:      revision.CreatedAt, Origin: revision.Origin,
			FileCount: revision.FileCount, Size: revision.Size,
			LastModified: syncStringPointer(revision.LastModified),
			Relation:     api.SyncHistoryRelation(revision.Relation),
		})
	}
	return c.JSON(http.StatusOK, response)
}

func (h SyncHandlers) HistoryDiff(c *echo.Context) error {
	h.restore()
	var request api.SyncHistoryDiffRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(request.Key) == 0 || len(request.Key) > 1024 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	diff, err := h.Service.DiffHistory(c.Request().Context(), key, request.Key)
	if err != nil {
		return syncProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SyncHistoryDiff{
		FromRevision: diff.FromRevision, ToRevision: diff.ToRevision,
		Added: diff.Added, Modified: diff.Modified, Removed: diff.Removed,
		DownloadedBytes: diff.DownloadedBytes,
	})
}

func syncStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// Pull は既定でプレビューし、求められたときだけ適用する。
//
// 応答が運ぶのはパスだけで、中身は決して運ばない。ファイルのバイト列を保持する pull
// の応答は、response body に秘密鍵を置くことになる。ユーザーが承認する
// ファイル単位のプレビューは、この application がすでに読めるファイルから組み立てられる。
func (h SyncHandlers) Pull(c *echo.Context) error {
	h.restore()
	var request api.PullRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if request.Apply != nil && *request.Apply &&
		(request.ExpectedETag == nil || request.ExpectedRevision == nil) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if (request.HistoryKey != nil && (len(*request.HistoryKey) == 0 || len(*request.HistoryKey) > 1024)) ||
		(request.ExpectedETag != nil && len(*request.ExpectedETag) > 1024) ||
		(request.ExpectedRevision != nil && !validSyncRevision(*request.ExpectedRevision)) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	resolve := remotesync.ResolveNone
	if request.Resolve != nil {
		switch *request.Resolve {
		case api.Local:
			resolve = remotesync.ResolveLocal
		case api.Remote:
			resolve = remotesync.ResolveRemote
		default:
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
	}
	historyKey := ""
	if request.HistoryKey != nil {
		historyKey = *request.HistoryKey
	}
	acceptRemoteHead := request.AcceptRemoteHead != nil && *request.AcceptRemoteHead
	if acceptRemoteHead && (h.Service.Direction() != remotesync.DirectionPull ||
		historyKey != "" || resolve != remotesync.ResolveRemote) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var result remotesync.PullResult
	var err error
	if request.Apply != nil && *request.Apply {
		if acceptRemoteHead {
			result, err = h.Service.PullAndApplyRemoteHeadUsing(c.Request().Context(), h.keyProvider(),
				*request.ExpectedETag, *request.ExpectedRevision)
		} else {
			result, err = h.Service.PullAndApplyUsing(c.Request().Context(), h.keyProvider(), resolve, historyKey,
				*request.ExpectedETag, *request.ExpectedRevision)
		}
	} else {
		key, ok, keyErr := h.sealingKey(c)
		if !ok {
			return keyErr
		}
		if acceptRemoteHead {
			result, err = h.Service.PullRemoteHead(c.Request().Context(), key)
		} else if historyKey == "" {
			result, err = h.Service.Pull(c.Request().Context(), key, resolve)
		} else {
			result, err = h.Service.PullHistory(c.Request().Context(), key, historyKey, resolve)
		}
	}
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		return syncKeyProblem(c, err)
	}

	response := api.PullResponse{
		Applied: false, Summary: snapshotSummaryResponse(result.Summary),
		DownloadedBytes: result.DownloadedBytes, CompletedAt: result.CompletedAt,
		Conflicts:  make([]api.SyncConflict, 0, len(result.Conflicts)),
		Written:    make([]string, 0, len(result.Request.Changes)),
		Removed:    make([]string, 0, len(result.Request.Removals)),
		RemoteETag: result.ETag, RemoteRevision: result.Manifest.Revision,
	}
	for _, conflict := range result.Conflicts {
		response.Conflicts = append(response.Conflicts, api.SyncConflict{
			Path:         conflict.Path,
			ChangedHere:  conflict.LocalDigest != "" && conflict.LocalDigest != conflict.BaseDigest,
			ChangedThere: conflict.RemoteDigest != conflict.BaseDigest,
		})
	}
	for _, change := range result.Request.Changes {
		response.Written = append(response.Written, h.Service.DisplayPath(change.Path))
	}
	for _, removal := range result.Request.Removals {
		response.Removed = append(response.Removed, h.Service.DisplayPath(removal.Path))
	}
	if result.Origin != "" {
		origin := result.Origin
		response.Origin = &origin
	}

	if request.Apply != nil && *request.Apply {
		response.Applied = true
		if h.Auto != nil {
			h.Auto.ManualApplyCompleted()
		}
		// Apply is a second request and therefore a second download. Its
		// completion is the point after the workspace transaction and state
		// write, not the earlier moment when this request finished downloading.
		state := h.Service.SyncState()
		if state.LastOperation != nil && state.LastOperation.Kind == remotesync.OperationApply {
			response.CompletedAt = state.LastOperation.CompletedAt
		}
	}
	return c.JSON(http.StatusOK, response)
}

func validSyncRevision(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

// safeObjectPath は bucket 名と同じくらい狭く絞ってあり、理由も同じである。
// パスはこの application が署名する URL のセグメントになるため、
// 独自のセグメントを足したり上位へ抜け出したりし得るものは、エスケープではなく拒否する。
func safeObjectPath(path string) bool {
	if path == "" {
		return true
	}
	if len(path) > 255 || strings.Contains(path, "..") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || !safeBucketName(segment) {
			return false
		}
	}
	return true
}

// safeBucketName はわざと狭く絞ってある。名前はこの application が
// 署名する URL のパスセグメントになるため、セグメントやクエリを
// 足し得るものは、エスケープではなく拒否する。
func safeBucketName(name string) bool {
	if name == "" || len(name) > 255 || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	for _, character := range name {
		switch {
		case character >= 'a' && character <= 'z',
			character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9',
			character == '-', character == '.', character == '_':
		default:
			return false
		}
	}
	return filepath.Base(name) == name
}

func syncProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, remotesync.ErrNotConfigured):
		return problem(c, http.StatusConflict, "sync_not_configured")
	case errors.Is(err, remotesync.ErrRemoteMoved):
		return problem(c, http.StatusConflict, "sync_remote_moved")
	case errors.Is(err, remotesync.ErrRemoteDeleted):
		return problem(c, http.StatusConflict, "sync_remote_deleted")
	case errors.Is(err, remotesync.ErrPreviewStale):
		return problem(c, http.StatusConflict, "preview_stale")
	case errors.Is(err, remotesync.ErrRecoveryRequired):
		return problem(c, http.StatusConflict, "sync_key_recovery_required")
	case errors.Is(err, remotesync.ErrRecoveryTargetChange):
		return problem(c, http.StatusConflict, "sync_key_recovery_target_change")
	case errors.Is(err, remotesync.ErrSetupTargetChanged):
		return problem(c, http.StatusConflict, "sync_setup_target_changed")
	case errors.Is(err, remotesync.ErrSetupTargetIncomplete):
		return problem(c, http.StatusConflict, "sync_setup_target_incomplete")
	case errors.Is(err, remotesync.ErrHistoryKeyLossConfirmation):
		return problem(c, http.StatusConflict, "sync_history_key_loss_confirmation_required")
	case errors.Is(err, remotesync.ErrNothingToPush):
		return problem(c, http.StatusConflict, "sync_nothing_to_push")
	case errors.Is(err, remotesync.ErrNoSnapshot):
		return problem(c, http.StatusNotFound, "sync_no_snapshot")
	case errors.Is(err, remotesync.ErrConflicts):
		return problem(c, http.StatusConflict, "sync_conflicts")
	case errors.Is(err, remotesync.ErrPushRefused):
		return problem(c, http.StatusConflict, "sync_push_refused")
	case errors.Is(err, remotesync.ErrApplyRefused):
		return problem(c, http.StatusConflict, "sync_apply_refused")
	case errors.Is(err, remotesync.ErrForcePushTarget):
		return problem(c, http.StatusBadRequest, "sync_force_target_invalid")
	case errors.Is(err, remotesync.ErrHistoryTarget):
		return problem(c, http.StatusBadRequest, "sync_history_target_invalid")
	case errors.Is(err, remotesync.ErrCommitMessage):
		return problem(c, http.StatusBadRequest, "sync_commit_message_invalid")
	case errors.Is(err, remotesync.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.Is(err, remotesync.ErrWeakPassphrase):
		return problem(c, http.StatusBadRequest, "passphrase_too_short")
	case errors.Is(err, remotesync.ErrCostRefused):
		return problem(c, http.StatusConflict, "snapshot_cost_refused")
	case errors.Is(err, remotesync.ErrUnsupportedEnvelopeVersion), errors.Is(err, remotesync.ErrUnsupportedVersion):
		return problem(c, http.StatusConflict, "snapshot_schema_unsupported")
	case errors.Is(err, remotesync.ErrUnsafePath), errors.Is(err, remotesync.ErrUnsafeMode),
		errors.Is(err, remotesync.ErrManifestMismatch), errors.Is(err, remotesync.ErrNotASnapshot):
		return problem(c, http.StatusConflict, "snapshot_rejected")
	case errors.Is(err, remotesync.ErrRefused), errors.Is(err, remotesync.ErrInsecureEndpoint):
		return problem(c, http.StatusBadGateway, "bucket_refused")
	case remotesync.IsLocalChange(err):
		return problem(c, http.StatusConflict, "sync_local_changed")
	case errors.Is(err, remotesync.ErrWorkspaceBusy):
		return problem(c, http.StatusConflict, "sync_workspace_busy")
	default:
		return problem(c, http.StatusBadGateway, "sync_failed")
	}
}

// keyConfigured は、vault から値を公開せず、同期鍵の設定有無だけを返す。
func (h SyncHandlers) keyConfigured() bool {
	if h.Secrets == nil {
		return false
	}
	settings, err := h.Secrets.SyncSettings()
	return err == nil && settings.Key != ""
}

// autoResponse は、巡回の現在地を画面の形にする。
func (h SyncHandlers) autoResponse() api.AutoSync {
	if h.Auto == nil {
		return api.AutoSync{Enabled: false, Phase: api.AutoSyncPhase(remotesync.AutoIdle)}
	}
	view := h.Auto.View()
	response := api.AutoSync{Enabled: view.Enabled, Phase: api.AutoSyncPhase(view.Phase)}
	if view.Detail != "" {
		detail := view.Detail
		response.Detail = &detail
	}
	if view.At != "" {
		at := view.At
		response.At = &at
	}
	return response
}

// SetAuto は、巡回の入切を決める。
//
// 切ったことも保管庫の中に残る。この実行のあいだだけ止まる切り方は、次に
// 起動したときに暗黙に再開することであり、止めたユーザーはそれを止めたと思っている。
func (h SyncHandlers) SetAuto(c *echo.Context) error {
	var request api.AutoSyncRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if h.Secrets == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	if err := h.Secrets.SetSyncAuto(request.Enabled); err != nil {
		if errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
	if request.Enabled && h.Auto != nil {
		h.restore()
		h.Auto.Now(c.Request().Context())
	}
	return h.status(c)
}

// Now は、一巡を押したユーザーを待たせたまま行う。
//
// 自動同期の入切とは独立した、明示的な一巡である。
func (h SyncHandlers) Now(c *echo.Context) error {
	h.restore()
	if h.Auto == nil {
		return problem(c, http.StatusConflict, "auto_sync_off")
	}
	h.Auto.Now(c.Request().Context())
	return h.status(c)
}
