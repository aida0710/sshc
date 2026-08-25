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

func addSyncActions(registry actionRegistry, service *remotesync.Service) {
	registry[session.ActionSyncForcePush] = actionKind{
		evidence: func(target string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		direction = remotesync.DirectionBoth
	}
	credentials := remotesync.Credentials{
		AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
	}
	config := remotesync.Config{
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path,
		Region: settings.Region, Direction: direction,
	}
	h.Service.Configure(config, credentials, remotesync.NewClient(config, credentials))
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
	parsed, err := url.Parse(request.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return problem(c, http.StatusBadRequest, "endpoint_must_be_https")
	}
	// client はパス全体を /bucket/key に置き換えるため、貼り付けられた
	// ".../my-bucket" は何も言わずに捨てられてしまい、ユーザーはこの
	// application が一度も書いたことのない場所にオブジェクトを探すことになる。
	// 末尾のスラッシュ 1 つはブラウザがホストに付け足すだけで意味を
	// 持たないため、拒否ではなく除去する。除去することで、画面が
	// "https://host//bucket" と表示するのも防いでいる。
	if strings.Trim(parsed.Path, "/") != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return problem(c, http.StatusBadRequest, "endpoint_must_have_no_path")
	}
	endpoint := strings.TrimRight(request.Endpoint, "/")
	if !safeBucketName(request.Bucket) {
		return problem(c, http.StatusBadRequest, "unsafe_bucket_name")
	}
	region := "auto"
	if request.Region != nil && *request.Region != "" {
		region = *request.Region
	}
	direction := remotesync.DirectionBoth
	if request.Direction != nil {
		parsed, ok := remotesync.ParseDirection(string(*request.Direction))
		if !ok {
			return problem(c, http.StatusBadRequest, "unknown_sync_direction")
		}
		direction = parsed
	}

	credentials := remotesync.Credentials{
		AccessKeyID:     request.AccessKeyId,
		SecretAccessKey: request.SecretAccessKey,
	}
	path := ""
	if request.Path != nil {
		path = strings.Trim(*request.Path, "/")
	}
	if !safeObjectPath(path) {
		return problem(c, http.StatusBadRequest, "unsafe_object_path")
	}
	config := remotesync.Config{
		Endpoint: endpoint, Bucket: request.Bucket, Path: path, Region: region, Direction: direction,
	}
	// 保存する前に試す。一度も試されなかった設定は、typo が最初の
	// push で何時間も後に別の場所で表面化する設定になってしまう。
	// ここは、ユーザーが自分の打ったものをまだ見られる唯一の画面である。
	client := remotesync.NewClient(config, credentials)
	if err := h.reach(c.Request().Context(), client, remotesync.ObjectKeyFor(config)); err != nil {
		return syncProblem(c, err)
	}

	// 使われる前に保存する。これにより、次の実行では消えているはずの
	// 設定を使ったと応答が主張してしまうことはない。
	if h.Secrets != nil {
		if err := h.Secrets.SetSyncSettings(secret.SyncSettings{
			Endpoint: endpoint, Bucket: request.Bucket, Path: path, Region: region,
			AccessKeyID: request.AccessKeyId, SecretAccessKey: request.SecretAccessKey,
			Direction: string(direction),
		}); err != nil {
			if errors.Is(err, secret.ErrLocked) {
				return problem(c, http.StatusConflict, "vault_locked")
			}
			return problem(c, http.StatusInternalServerError, "vault_failed")
		}
	}
	h.Service.Configure(config, credentials, client)
	return h.status(c)
}

// sealingKey は、vault に保存した同期専用の暗号鍵を返す。
// 取得できない場合は HTTP 応答を書き込み、ok=false を返す。
func (h SyncHandlers) sealingKey(c *echo.Context) (string, bool, error) {
	if h.Secrets == nil {
		return "", false, problem(c, http.StatusConflict, "sync_key_missing")
	}
	settings, err := h.Secrets.SyncSettings()
	switch {
	case errors.Is(err, secret.ErrLocked), errors.Is(err, secret.ErrNoVault):
		return "", false, problem(c, http.StatusConflict, "vault_locked")
	case err != nil:
		return "", false, problem(c, http.StatusInternalServerError, "vault_unreadable")
	case settings.Key == "":
		return "", false, problem(c, http.StatusConflict, "sync_key_missing")
	}
	return settings.Key, true, nil
}

// SetKey は、このワークスペースが同期に使う鍵を決める。
//
// 本文が空なら作る。既定が生成であることに理由がある、この値は端末をまたいで
// 共有されるので、どこかに書き留められる。書き留めるなら、覚えられる必要はない。
//
// 応答は、採った鍵そのものである。平文でこれが出る唯一の場所であり、画面はこれを
// 一度だけ見せる。
func (h SyncHandlers) SetKey(c *echo.Context) error {
	var request api.SyncKeyRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if h.Secrets == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	key := ""
	if request.Key != nil {
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
	if err := h.Secrets.SetSyncKey(key); err != nil {
		if errors.Is(err, secret.ErrLocked) || errors.Is(err, secret.ErrNoVault) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
	return c.JSON(http.StatusOK, api.SyncKeyResponse{Key: key})
}

func (h SyncHandlers) Push(c *echo.Context) error {
	h.restore()
	var request api.SyncPushRequest
	if c.Request().ContentLength != 0 {
		if err := decodeJSON(c, &request); err != nil {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
	}
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	message := ""
	if request.Message != nil {
		message = *request.Message
	}
	result, err := h.Service.PushWithMessage(c.Request().Context(), key, message)
	if err != nil {
		return syncProblem(c, err)
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
	if c.Request().ContentLength != 0 {
		if err := decodeJSON(c, &request); err != nil {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
	}
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	confirmation, err := h.Service.ForcePushConfirmation(c.Request().Context(), remotesync.ForcePushTarget)
	if err != nil {
		return syncProblem(c, err)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSyncForcePush,
		remotesync.ForcePushTarget, confirmation.Evidence); !allowed {
		return response
	}
	message := ""
	if request.Message != nil {
		message = *request.Message
	}
	result, err := h.Service.ForcePushWithMessage(c.Request().Context(), key, confirmation.ETag, message)
	if err != nil {
		return syncProblem(c, err)
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
		Revisions:       make([]api.SyncHistoryRevision, 0, len(view.Revisions)),
	}
	for _, revision := range view.Revisions {
		response.Revisions = append(response.Revisions, api.SyncHistoryRevision{
			Key: revision.Key, Revision: revision.Revision,
			ParentRevision: syncStringPointer(revision.ParentRevision),
			Message:        syncStringPointer(revision.Message),
			CreatedAt:      revision.CreatedAt, Origin: revision.Origin,
			FileCount: revision.FileCount, Size: revision.Size,
			LastModified: syncStringPointer(revision.LastModified),
			Relation:     api.SyncHistoryRelation(revision.Relation), Legacy: revision.Legacy,
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
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
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
	var result remotesync.PullResult
	if request.HistoryKey == nil {
		result, err = h.Service.Pull(c.Request().Context(), key, resolve)
	} else {
		result, err = h.Service.PullHistory(c.Request().Context(), key, *request.HistoryKey, resolve)
	}
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		return syncProblem(c, err)
	}

	response := api.PullResponse{
		Applied: false, Summary: snapshotSummaryResponse(result.Summary),
		DownloadedBytes: result.DownloadedBytes, CompletedAt: result.CompletedAt,
		Conflicts: make([]api.SyncConflict, 0, len(result.Conflicts)),
		Written:   make([]string, 0, len(result.Request.Changes)),
		Removed:   make([]string, 0, len(result.Request.Removals)),
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
		if err := h.Service.Apply(result); err != nil {
			return syncProblem(c, err)
		}
		response.Applied = true
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
		return problem(c, http.StatusConflict, "snapshot_too_new")
	case errors.Is(err, remotesync.ErrUnsafePath), errors.Is(err, remotesync.ErrUnsafeMode),
		errors.Is(err, remotesync.ErrManifestMismatch), errors.Is(err, remotesync.ErrNotASnapshot):
		return problem(c, http.StatusConflict, "snapshot_rejected")
	case errors.Is(err, remotesync.ErrRefused), errors.Is(err, remotesync.ErrInsecureEndpoint):
		return problem(c, http.StatusBadGateway, "bucket_refused")
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
	return h.status(c)
}

// Now は、一巡を押したユーザーを待たせたまま行う。
//
// 巡回が入っていなければ何も起きない。「今すぐ」は自動同期の一部であって、
// 手動の push と pull はそれぞれのボタンが持っている。
func (h SyncHandlers) Now(c *echo.Context) error {
	h.restore()
	if h.Auto == nil {
		return problem(c, http.StatusConflict, "auto_sync_off")
	}
	h.Auto.Once(c.Request().Context())
	return h.status(c)
}
