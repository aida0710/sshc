package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/secret"
)

const (
	VaultStatusPath = "/cli/vault/status"
	VaultCreatePath = "/cli/vault/create"
	VaultVerifyPath = "/cli/vault/verify"
	VaultUnlockPath = "/cli/vault/unlock"
	VaultLockPath   = "/cli/vault/lock"
	VaultChangePath = "/cli/vault/change-password"
)

const maxVaultCLIBody = 4 << 10

type vaultPassphraseRequest struct {
	Passphrase *string `json:"passphrase"`
}

type vaultChangeRequest struct {
	Current *string `json:"current"`
	Next    *string `json:"next"`
}

// registerVaultCLIRoutes は、handoff の秘密を確かめる /cli の group に Vault の操作を登録する。
func registerVaultCLIRoutes(authenticated *echo.Group, handlers CLIHandlers) {
	authenticated.GET(cliRoute(VaultStatusPath), handlers.VaultStatus)
	authenticated.POST(cliRoute(VaultCreatePath), handlers.VaultCreate)
	authenticated.POST(cliRoute(VaultUnlockPath), handlers.VaultUnlock)
	authenticated.POST(cliRoute(VaultVerifyPath), handlers.VaultVerify)
	authenticated.POST(cliRoute(VaultLockPath), handlers.VaultLock)
	authenticated.POST(cliRoute(VaultChangePath), handlers.VaultChange)
}

// decodeVaultCLIJSON は decodeJSONWithin の規則で読み、断るときの状態コードを返す。
// 読めたときは 0 を返す。CLI は大きすぎるボディを 413 で見分ける。
func decodeVaultCLIJSON(c *echo.Context, target any) int {
	err := decodeJSONWithin(c, maxVaultCLIBody, target)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errBodyTooLarge):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusBadRequest
	}
}

func (h CLIHandlers) VaultStatus(c *echo.Context) error {
	answer, err := h.cliStatus()
	if err != nil {
		return unexpectedNoContent(c, err)
	}
	return c.JSON(http.StatusOK, answer)
}

func (h CLIHandlers) VaultCreate(c *echo.Context) error {
	var request vaultPassphraseRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if request.Passphrase == nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if err := withVault(h.Vault, func(vault *secret.Service) error {
		return vault.Initialise(*request.Passphrase)
	}); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h CLIHandlers) VaultUnlock(c *echo.Context) error {
	var request vaultPassphraseRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if request.Passphrase == nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if err := withVault(h.Vault, func(vault *secret.Service) error {
		return vault.Unlock(*request.Passphrase)
	}); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// VaultVerify checks the current password without changing the unlocked state.
func (h CLIHandlers) VaultVerify(c *echo.Context) error {
	var request vaultPassphraseRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if request.Passphrase == nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if err := withVault(h.Vault, func(vault *secret.Service) error {
		valid, err := vault.Verify(*request.Passphrase)
		if err == nil && !valid {
			return secret.ErrWrongPassphrase
		}
		return err
	}); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h CLIHandlers) VaultLock(c *echo.Context) error {
	var request struct{}
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	// session と vault は別の寿命を持つ。ここで触るのは導出済みの vault key だけである。
	if err := withVault(h.Vault, (*secret.Service).LockManually); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h CLIHandlers) VaultChange(c *echo.Context) error {
	var request vaultChangeRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if request.Current == nil || request.Next == nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if err := withVault(h.Vault, func(vault *secret.Service) error {
		return vault.ChangeMasterPassword(c.Request().Context(), *request.Current, *request.Next)
	}); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// vaultCLIProblem は、CLI の Vault の route の拒否を本文なしの状態コードにする。
//
// 中断した変更と workspace の busy は、ブラウザの handler と同じく boundaryRefusalFor で
// 409 にし、想定外の失敗として記録しない。/cli/ の route は本文を返さないので、CLI は
// この 2 つをほかの 409 と見分けられず、中断した変更を History で復旧する案内も出せない。
func vaultCLIProblem(c *echo.Context, err error) error {
	if refusal, ok := boundaryRefusalFor(err); ok {
		return c.NoContent(refusal.status)
	}
	switch {
	case errors.Is(err, secret.ErrAlreadyExists), errors.Is(err, secret.ErrNoVault), errors.Is(err, secret.ErrLocked):
		return c.NoContent(http.StatusConflict)
	case errors.Is(err, secret.ErrWrongPassphrase):
		return c.NoContent(http.StatusUnauthorized)
	case errors.Is(err, secret.ErrWeakPassphrase):
		return c.NoContent(http.StatusBadRequest)
	default:
		return unexpectedNoContent(c, err)
	}
}
