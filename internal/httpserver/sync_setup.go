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

// Bucket setup: validating the target and proving it is reachable before
// anything is stored.

func syncSetupInput(request api.SyncSetupCheckRequest, credentials remotesync.Credentials) (remotesync.Config, remotesync.Credentials, error) {
	if len(credentials.AccessKeyID) == 0 || len(credentials.AccessKeyID) > remotesync.MaxAccessKeyIDLength ||
		len(credentials.SecretAccessKey) == 0 || len(credentials.SecretAccessKey) > remotesync.MaxSecretAccessKeyLength {
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
		if accessKeyID != nil || secretAccessKey != nil || h.Vault == nil {
			return remotesync.Credentials{}, errSyncSetupInvalidRequest
		}
		settings, err := h.Vault.SyncSettings()
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
	return unexpectedProblem(c, "vault_unreadable", err)
}

func setupInputProblem(c *echo.Context, err error) error {
	for _, refusal := range remotesync.TargetRefusals {
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
	inspection, err := remotesync.InspectSetupTarget(c.Request().Context(), h.objectStoreClient(config, credentials), config)
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
	if h.Vault == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	syncKey := strings.TrimSpace(request.Key)
	generated := false
	if request.ReuseKey {
		if syncKey != "" || request.ExpectedState != api.Existing {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		syncKey, err = h.currentSyncKey()
		if err != nil {
			return syncKeyProblem(c, err)
		}
	} else if syncKey == "" {
		if request.ExpectedState != api.Empty {
			return problem(c, http.StatusBadRequest, "sync_key_missing")
		}
		syncKey, err = remotesync.NewKey()
		if err != nil {
			return unexpectedProblem(c, "key_generation_failed", err)
		}
		generated = true
	}
	if len(syncKey) > remotesync.MaxKeyLength {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	expected := remotesync.SetupInspection{
		State: remotesync.SetupTargetState(request.ExpectedState), HistoryPresent: request.HistoryPresent,
	}
	if request.ExpectedETag != nil {
		expected.ETag = *request.ExpectedETag
	}
	client := h.objectStoreClient(config, credentials)
	err = h.Service.CompleteSetup(c.Request().Context(), config, credentials, client, expected, syncKey, func() error {
		return h.Vault.SetSyncSettings(secret.SyncSettings{
			Endpoint: config.Endpoint, Bucket: config.Bucket, Path: config.Path, Region: config.Region,
			AccessKeyID: credentials.AccessKeyID, SecretAccessKey: credentials.SecretAccessKey,
			Direction: string(direction), Key: syncKey,
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
		response.GeneratedKey = &syncKey
	}
	return c.JSON(http.StatusOK, response)
}
