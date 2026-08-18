package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/envelope"
	"sshc/internal/objectstore"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// SyncHandlers はリモートのスナップショットを提供する。
type SyncHandlers struct {
	Service *remotesync.Service
	// Secrets はマスターパスワードで封印された object store の設定を
	// 保持し、vault の中ではなく脇に置く——vault は持ち運ばれるものであり、
	// bucket への鍵が bucket の中にあってはならないからだ。nil の場合、
	// 設定は実行のたびのものになる。これは保存される前にしていたことである。
	Secrets *secret.Service
	// Reach は設定が保存される前にそれが機能するかを尋ねる。これが
	// 注入されているのは、設定を保存するのはこのハンドラの仕事だが
	// bucket に到達することはそうではないからで、またこのパッケージの
	// どのテストもネットワークに触れてはならないからだ。nil の場合は
	// 実際のチェックを意味する。配線し忘れが黙って「問題なし」になってはならない。
	Reach func(ctx context.Context, client *objectstore.Client, key string) error
	// Auto は、押さなくても進む巡回である。nil のときは、この設置に巡回が無い
	// ——status は「入っていない」と答え、押しても一巡しない。
	Auto *remotesync.Auto
}

func (h SyncHandlers) reach(ctx context.Context, client *objectstore.Client, key string) error {
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
	engine.POST("/api/v1/sync/rekey", handlers.Rekey)
	engine.POST("/api/v1/sync/push", handlers.Push)
	engine.POST("/api/v1/sync/pull", handlers.Pull)
}

// restore は保存された設定から client を構成する。解錠さえすれば
// 画面が埋まり push が動くようになるということだ。
//
// ここでは施錠中の vault はエラーではない。status がそう伝え、form も
// なぜ空なのかを伝える。起動時には何も尋ねない。これは画面が必要な
// ときに自ら尋ねる、というだけのことである。
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
	credentials := objectstore.Credentials{
		AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
	}
	config := remotesync.Config{
		Endpoint: settings.Endpoint, Bucket: settings.Bucket, Path: settings.Path,
		Region: settings.Region, Direction: direction,
	}
	h.Service.Configure(config, credentials, &objectstore.Client{
		Endpoint: config.Endpoint, Bucket: config.Bucket, Region: config.Region, Creds: credentials,
	})
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
		// form が空である理由を、空であるときに伝える。access key や
		// secret は決して含まない。それらは封印されたファイルへ一方通行である。
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
// credentials はマスターパスワードで封印され、vault の中ではなく脇に
// 置かれる。vault は持ち運ばれる——Collect は sshc/secrets をはっきり
// 名指ししている——ので、自分自身の bucket への鍵を運ぶスナップショットは、
// 便利なブートストラップである以上にはるかに大きな被害範囲になる。
// 誰かが 1 つのスナップショットを手に入れれば、それ以降のすべてを取得できてしまうからだ。
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
	// 持たないため、拒否ではなく除去する——除去することで、画面が
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

	credentials := objectstore.Credentials{
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
	client := &objectstore.Client{
		Endpoint: config.Endpoint, Bucket: config.Bucket, Region: config.Region, Creds: credentials,
	}
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

// sealingKey は、リモートのスナップショットを封じている値を保管庫から取り出す。
//
// **かつてここはマスターパスワードを検査していた。** 打ち間違いが誰にも開けない
// 書庫を作るのを防ぐためであり、その目的には正しく働いていた。しかし副作用として、
// マスターパスワードが端末をまたいだ共有秘密になっていた——1 台で変えれば他の
// 全端末が締め出され、打つ人が居ないところでは封を開けられなかった。
//
// 鍵は保管庫の中にある。**保管庫が開いていることが、同期してよいことの唯一の条件
// である**——開けられるのはマスターパスワードを知っている人だけなのだから、
// 権限としてはこれで足りている。
//
// 先へ進めない場合はすでに拒否を書き込んでおり、返すエラーは呼び出し元がそのまま
// 返すべきものである——nil である。レスポンスを書くことがこの application の
// 拒否の仕方だからだ。そのエラーだけを返せば、呼び出し元は今拒否したはずのことを、
// 自分の答えの上から続けてしまいかねない。
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
// 本文が空なら作る。**既定が生成であることに理由がある** — この値は端末をまたいで
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
	// 弱い鍵は、封をする段になって初めて分かるのでは遅い。ここで断る。
	if _, err := envelope.Derive(key); err != nil {
		if errors.Is(err, envelope.ErrWeakPassphrase) {
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

// Rekey は、古い鍵で封じられたリモートを、いまの鍵で開くようにする。
//
// これは移行のためだけにある。ワークスペースには 1 バイトも触れない。
func (h SyncHandlers) Rekey(c *echo.Context) error {
	h.restore()
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	if _, err := h.Service.Rekey(c.Request().Context(), request.Passphrase, key); err != nil {
		return syncProblem(c, err)
	}
	return h.status(c)
}

func (h SyncHandlers) Push(c *echo.Context) error {
	h.restore()
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	result, err := h.Service.Push(c.Request().Context(), key)
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
	result, err := h.Service.Pull(c.Request().Context(), key, resolve)
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
	case errors.Is(err, envelope.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.Is(err, envelope.ErrWeakPassphrase):
		return problem(c, http.StatusBadRequest, "passphrase_too_short")
	case errors.Is(err, envelope.ErrCostRefused):
		return problem(c, http.StatusConflict, "snapshot_cost_refused")
	case errors.Is(err, envelope.ErrUnsupportedVersion), errors.Is(err, remotesync.ErrUnsupportedVersion):
		return problem(c, http.StatusConflict, "snapshot_too_new")
	case errors.Is(err, remotesync.ErrUnsafePath), errors.Is(err, remotesync.ErrUnsafeMode),
		errors.Is(err, remotesync.ErrManifestMismatch), errors.Is(err, remotesync.ErrNotASnapshot):
		return problem(c, http.StatusConflict, "snapshot_rejected")
	case errors.Is(err, objectstore.ErrRefused), errors.Is(err, objectstore.ErrInsecureEndpoint):
		return problem(c, http.StatusBadGateway, "bucket_refused")
	default:
		return problem(c, http.StatusBadGateway, "sync_failed")
	}
}

// keyConfigured は、同期の鍵が保管庫に在るかを答える。
//
// **値そのものは返さない。** リモートを開ける値を、状態を読むたびに配る API に
// してはならない。施錠中は「無い」と答える——読めないものについて、在るとも
// 無いとも言えないが、画面がまず言うべきことは施錠されていることであり、それは
// 別のフィールドが言っている。
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
// **切ったことも保管庫の中に残る。** この実行のあいだだけ止まる切り方は、次に
// 起こしたときに黙って再開することであり、止めた人はそれを止めたと思っている。
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

// Now は、一巡を押した人を待たせたまま行う。
//
// 巡回が入っていなければ何も起きない——「今すぐ」は自動同期の一部であって、
// 手動の push と pull はそれぞれのボタンが持っている。
func (h SyncHandlers) Now(c *echo.Context) error {
	h.restore()
	if h.Auto == nil {
		return problem(c, http.StatusConflict, "auto_sync_off")
	}
	h.Auto.Once(c.Request().Context())
	return h.status(c)
}
