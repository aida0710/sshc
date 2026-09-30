package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// The shared key that seals snapshots: reading it from the vault for a
// request, and creating or replacing it.

// requireSyncKeyは、vaultに保存した同期鍵を返す。
// 取得できない場合は HTTP 応答を書き込み、ok=false を返す。
func (h SyncHandlers) requireSyncKey(c *echo.Context) (string, bool, error) {
	syncKey, err := h.currentSyncKey()
	switch {
	case vaultUnavailable(err):
		return "", false, problem(c, http.StatusConflict, "vault_locked")
	case errors.Is(err, errSyncKeyMissing):
		return "", false, problem(c, http.StatusConflict, "sync_key_missing")
	case err != nil:
		return "", false, unexpectedProblem(c, "vault_unreadable", err)
	}
	return syncKey, true, nil
}

func (h SyncHandlers) currentSyncKey() (string, error) {
	if h.Vault == nil {
		return "", errSyncKeyMissing
	}
	settings, err := h.Vault.SyncSettings()
	if err != nil {
		return "", err
	}
	if settings.Key == "" {
		return "", errSyncKeyMissing
	}
	return settings.Key, nil
}

func (h SyncHandlers) syncKeyProvider() remotesync.KeyProvider {
	return h.currentSyncKey
}

func syncKeyProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, errSyncKeyMissing):
		return problem(c, http.StatusConflict, "sync_key_missing")
	case vaultUnavailable(err):
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
	if h.Vault == nil {
		return problem(c, http.StatusConflict, "vault_locked")
	}
	syncKey := ""
	if request.Key != nil {
		if len(*request.Key) == 0 || len(*request.Key) > remotesync.MaxKeyLength {
			return problem(c, http.StatusBadRequest, "invalid_request")
		}
		syncKey = strings.TrimSpace(*request.Key)
	}
	if syncKey == "" {
		generated, err := remotesync.NewKey()
		if err != nil {
			return unexpectedProblem(c, "key_generation_failed", err)
		}
		syncKey = generated
	}
	// 弱い鍵は、暗号化する段になって初めて分かるのでは遅い。ここで断る。
	if err := remotesync.ValidateKey(syncKey); err != nil {
		if errors.Is(err, remotesync.ErrWeakPassphrase) {
			return problem(c, http.StatusBadRequest, "passphrase_too_short")
		}
		return unexpectedProblem(c, "vault_unreadable", err)
	}
	confirmHistoryLoss := request.ConfirmHistoryLoss != nil && *request.ConfirmHistoryLoss
	if err := h.Service.ReplaceKeyUsing(c.Request().Context(), syncKey, confirmHistoryLoss, func() (string, func() error, error) {
		settings, err := h.Vault.SyncSettings()
		if err != nil {
			return "", nil, err
		}
		commit := func() error {
			if err := h.Vault.SetSyncKeyIfSettingsMatch(settings, syncKey); errors.Is(err, secret.ErrSyncSettingsChanged) {
				return remotesync.ErrRemoteMoved
			} else {
				return err
			}
		}
		return settings.Key, commit, nil
	}); err != nil {
		if vaultUnavailable(err) {
			return problem(c, http.StatusConflict, "vault_locked")
		}
		if errors.Is(err, remotesync.ErrRemoteMoved) || errors.Is(err, remotesync.ErrWrongPassphrase) ||
			errors.Is(err, remotesync.ErrUnsupportedEnvelopeVersion) || errors.Is(err, remotesync.ErrUnsupportedVersion) ||
			errors.Is(err, remotesync.ErrNotASnapshot) || errors.Is(err, remotesync.ErrRecoveryRequired) ||
			errors.Is(err, remotesync.ErrHistoryKeyLossConfirmation) {
			return syncProblem(c, err)
		}
		return unexpectedProblem(c, "vault_failed", err)
	}
	return c.JSON(http.StatusOK, api.SyncKeyResponse{Key: syncKey})
}

// keyConfigured は、vault から値を公開せず、同期鍵の設定有無だけを返す。
func (h SyncHandlers) keyConfigured() bool {
	if h.Vault == nil {
		return false
	}
	settings, err := h.Vault.SyncSettings()
	return err == nil && settings.Key != ""
}
