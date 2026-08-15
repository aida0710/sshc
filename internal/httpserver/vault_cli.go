package httpserver

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/handoff"
	"sshc/internal/secret"
)

const (
	VaultStatusPath = "/cli/vault/status"
	VaultCreatePath = "/cli/vault/create"
	VaultUnlockPath = "/cli/vault/unlock"
	VaultLockPath   = "/cli/vault/lock"
	VaultChangePath = "/cli/vault/change-password"
)

const maxVaultCLIBody = 4 << 10

type vaultPassphraseRequest struct {
	Passphrase string `json:"passphrase"`
}

type vaultChangeRequest struct {
	Current string `json:"current"`
	Next    string `json:"next"`
}

func registerVaultCLIRoutes(engine *echo.Echo, handlers ConnectHandlers) {
	engine.GET(VaultStatusPath, handlers.VaultStatus)
	engine.POST(VaultCreatePath, handlers.VaultCreate)
	engine.POST(VaultUnlockPath, handlers.VaultUnlock)
	engine.POST(VaultLockPath, handlers.VaultLock)
	engine.POST(VaultChangePath, handlers.VaultChange)
}

// cliAuthorised は同じ長さの handoff secret を constant-time で比較する。
func cliAuthorised(request *http.Request, expected string) bool {
	presented := request.Header.Get(handoff.HeaderName)
	return expected != "" && len(presented) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}

func decodeVaultCLIJSON(c *echo.Context, target any) int {
	request := c.Request()
	if request.Body == nil {
		return http.StatusBadRequest
	}
	if request.ContentLength > maxVaultCLIBody {
		return http.StatusRequestEntityTooLarge
	}
	limited := http.MaxBytesReader(c.Response(), request.Body, maxVaultCLIBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return http.StatusRequestEntityTooLarge
		}
		return http.StatusBadRequest
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return http.StatusBadRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return http.StatusBadRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return http.StatusBadRequest
	}
	return 0
}

func (h ConnectHandlers) vaultAuthorised(c *echo.Context) bool {
	return cliAuthorised(c.Request(), h.Secret)
}

func (h ConnectHandlers) VaultStatus(c *echo.Context) error {
	if !h.vaultAuthorised(c) {
		return c.NoContent(http.StatusUnauthorized)
	}
	answer, err := h.cliStatus()
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.JSON(http.StatusOK, answer)
}

func (h ConnectHandlers) VaultCreate(c *echo.Context) error {
	if !h.vaultAuthorised(c) {
		return c.NoContent(http.StatusUnauthorized)
	}
	var request vaultPassphraseRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if err := h.vault.Initialise(request.Passphrase); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h ConnectHandlers) VaultUnlock(c *echo.Context) error {
	if !h.vaultAuthorised(c) {
		return c.NoContent(http.StatusUnauthorized)
	}
	var request vaultPassphraseRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	if err := h.vault.Unlock(request.Passphrase); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h ConnectHandlers) VaultLock(c *echo.Context) error {
	if !h.vaultAuthorised(c) {
		return c.NoContent(http.StatusUnauthorized)
	}
	var request struct{}
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	// session と vault は別の寿命を持つ。ここで触るのは導出済みの vault key だけである。
	if err := h.vault.Lock(); err != nil {
		return vaultCLIProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h ConnectHandlers) VaultChange(c *echo.Context) error {
	if !h.vaultAuthorised(c) {
		return c.NoContent(http.StatusUnauthorized)
	}
	var request vaultChangeRequest
	if status := decodeVaultCLIJSON(c, &request); status != 0 {
		return c.NoContent(status)
	}
	result, err := h.vault.Change(c.Request().Context(), request.Current, request.Next)
	if err != nil {
		return vaultCLIProblem(c, err)
	}
	if result.SnapshotProblem != nil {
		return c.JSON(http.StatusMultiStatus, result)
	}
	return c.NoContent(http.StatusNoContent)
}

func vaultCLIProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, secret.ErrAlreadyExists), errors.Is(err, secret.ErrNoVault), errors.Is(err, secret.ErrLocked):
		return c.NoContent(http.StatusConflict)
	case errors.Is(err, secret.ErrWrongPassphrase):
		return c.NoContent(http.StatusUnauthorized)
	case errors.Is(err, secret.ErrWeakPassphrase):
		return c.NoContent(http.StatusBadRequest)
	default:
		return c.NoContent(http.StatusInternalServerError)
	}
}
