package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// Bucket setup: validating the target, proving it is reachable before
// anything is stored, and the settings changes on a configured sync.

func syncSetupInput(request api.SyncSetupCheckRequest, credentials remotesync.Credentials) (remotesync.Config, remotesync.Credentials, error) {
	if len(credentials.AccessKeyID) == 0 || len(credentials.AccessKeyID) > 512 ||
		len(credentials.SecretAccessKey) == 0 || len(credentials.SecretAccessKey) > 512 {
		return remotesync.Config{}, remotesync.Credentials{}, remotesync.ErrTargetTooLong
	}
	input := remotesync.TargetInput{Endpoint: request.Endpoint, Bucket: request.Bucket, Region: "auto"}
	if request.Path != nil {
		input.Path = *request.Path
	}
	if request.Region != nil && *request.Region != "" {
		input.Region = *request.Region
	}
	target, err := remotesync.ValidateTarget(input)
	if err != nil {
		return remotesync.Config{}, remotesync.Credentials{}, err
	}
	return remotesync.Config{
		Endpoint: target.Endpoint, Bucket: target.Bucket, Path: target.Path, Region: target.Region,
		Direction: remotesync.DirectionBoth,
	}, credentials, nil
}

func (h SyncHandlers) setupCredentials(
	reuse bool, accessKeyID, secretAccessKey *string,
) (remotesync.Credentials, error) {
	if reuse {
		if accessKeyID != nil || secretAccessKey != nil || h.Secrets == nil {
			return remotesync.Credentials{}, errSyncSetupInvalidRequest
		}
		settings, err := h.Secrets.SyncSettings()
		if err != nil {
			return remotesync.Credentials{}, err
		}
		if settings.AccessKeyID == "" || settings.SecretAccessKey == "" {
			return remotesync.Credentials{}, errSyncSetupInvalidRequest
		}
		return remotesync.Credentials{
			AccessKeyID: settings.AccessKeyID, SecretAccessKey: settings.SecretAccessKey,
		}, nil
	}
	if accessKeyID == nil || secretAccessKey == nil {
		return remotesync.Credentials{}, errSyncSetupInvalidRequest
	}
	return remotesync.Credentials{
		AccessKeyID: *accessKeyID, SecretAccessKey: *secretAccessKey,
	}, nil
}

func setupCredentialsProblem(c *echo.Context, err error) error {
	if vaultUnavailable(err) {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	if errors.Is(err, errSyncSetupInvalidRequest) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	return problem(c, http.StatusInternalServerError, "vault_unreadable")
}

func setupInputProblem(c *echo.Context, err error) error {
	for _, refusal := range []error{
		remotesync.ErrEndpointNotHTTPS, remotesync.ErrEndpointHasPath, remotesync.ErrUnsafeBucketName, remotesync.ErrUnsafeObjectPath,
	} {
		if errors.Is(err, refusal) {
			return problem(c, http.StatusBadRequest, refusal.Error())
		}
	}
	return problem(c, http.StatusBadRequest, "invalid_request")
}

// CheckSetup probes an exact destination without persisting credentials.
func (h SyncHandlers) CheckSetup(c *echo.Context) error {
	var request api.SyncSetupCheckRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	credentials, err := h.setupCredentials(request.ReuseCredentials, request.AccessKeyId, request.SecretAccessKey)
	if err != nil {
		return setupCredentialsProblem(c, err)
	}
	config, credentials, err := syncSetupInput(request, credentials)
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
	credentials, err := h.setupCredentials(request.ReuseCredentials, request.AccessKeyId, request.SecretAccessKey)
	if err != nil {
		return setupCredentialsProblem(c, err)
	}
	config, credentials, err := syncSetupInput(api.SyncSetupCheckRequest{
		Endpoint: request.Endpoint, Bucket: request.Bucket, Path: request.Path, Region: request.Region,
		ReuseCredentials: request.ReuseCredentials,
	}, credentials)
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
	if request.ReuseKey {
		if key != "" || request.ExpectedState != api.Existing {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		key, err = h.currentSyncKey()
		if err != nil {
			return syncKeyProblem(c, err)
		}
	} else if key == "" {
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
		if vaultUnavailable(err) {
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

// Configure はこのマシンをある bucket に向ける。
//
// credentials はマスターパスワードで暗号化し、同期対象外の専用ファイルへ保存する。
// 同期先の資格情報を同期スナップショット自体へ含めない。
func (h SyncHandlers) Configure(c *echo.Context) error {
	var request api.SyncSettingsRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	credentials := remotesync.Credentials{
		AccessKeyID: request.AccessKeyId, SecretAccessKey: request.SecretAccessKey,
	}
	config, credentials, err := syncSetupInput(api.SyncSetupCheckRequest{
		Endpoint: request.Endpoint, Bucket: request.Bucket, Path: request.Path, Region: request.Region,
	}, credentials)
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
			if vaultUnavailable(err) {
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
