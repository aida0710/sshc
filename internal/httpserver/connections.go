package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// ConnectionHandlers owns the creation boundary. It deliberately receives the
// current key inventory rather than accepting a path from the browser, so a
// request can only select a private key the server has just inventoried.
type ConnectionHandlers struct {
	Service *application.Service
	Secrets *secret.Service
	Keys    KeyService
}

func registerConnectionRoutes(engine *echo.Echo, handlers ConnectionHandlers) {
	engine.POST("/api/v1/connections", handlers.Create)
}

func (h ConnectionHandlers) Create(c *echo.Context) error {
	var wire api.CreateConnectionRequest
	if err := decodeBody(c, &wire); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	defer wipeBuffer(wire.Authentication)
	request, err := connectionRequestFromAPI(wire)
	if err != nil {
		return connectionProblem(c, err)
	}

	var inventory *keys.Inventory
	if request.Authentication.Kind == application.CreateAuthenticationIdentityFile {
		if h.Keys == nil {
			return problem(c, http.StatusInternalServerError, "inventory_failed")
		}
		inventory, err = h.Keys.Inventory()
		if err != nil {
			return problem(c, http.StatusInternalServerError, "inventory_failed")
		}
	}
	result, err := h.Service.CreateConnection(h.Secrets, inventory, request)
	if err != nil {
		return connectionProblem(c, err)
	}
	return c.JSON(http.StatusCreated, result)
}

func connectionRequestFromAPI(wire api.CreateConnectionRequest) (application.CreateConnectionRequest, error) {
	if len(wire.Alias) == 0 || len(wire.Alias) > 64 ||
		len(wire.HostName) == 0 || len(wire.HostName) > platform.MaxHostnameLength ||
		wire.Group != nil && len(*wire.Group) > 400 ||
		wire.User != nil && len(*wire.User) > 255 {
		return application.CreateConnectionRequest{}, errInvalidEdit
	}

	request := application.CreateConnectionRequest{
		Alias: wire.Alias, HostName: wire.HostName, Port: wire.Port,
	}
	if wire.Group != nil {
		request.Group = *wire.Group
	}
	if wire.User != nil {
		request.User = *wire.User
	}

	var tagged struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(wire.Authentication, &tagged); err != nil {
		return application.CreateConnectionRequest{}, errInvalidEdit
	}
	switch tagged.Kind {
	case string(application.CreateAuthenticationDedicatedPassword):
		var authentication api.CreateDedicatedPasswordAuthentication
		if err := decodeConnectionAuthentication(wire.Authentication, &authentication); err != nil ||
			authentication.Password == "" || len(authentication.Password) > 1024 {
			return application.CreateConnectionRequest{}, errInvalidEdit
		}
		request.Authentication = application.CreateAuthentication{
			Kind: application.CreateAuthenticationDedicatedPassword, Password: authentication.Password,
		}
	case string(application.CreateAuthenticationSavedPassword):
		var authentication api.CreateSavedPasswordAuthentication
		if err := decodeConnectionAuthentication(wire.Authentication, &authentication); err != nil ||
			authentication.Credential == "" || len(authentication.Credential) > 128 {
			return application.CreateConnectionRequest{}, errInvalidEdit
		}
		request.Authentication = application.CreateAuthentication{
			Kind: application.CreateAuthenticationSavedPassword, Credential: authentication.Credential,
		}
	case string(application.CreateAuthenticationNewSharedPassword):
		var authentication api.CreateNewSharedPasswordAuthentication
		if err := decodeConnectionAuthentication(wire.Authentication, &authentication); err != nil ||
			authentication.Credential == "" || len(authentication.Credential) > 128 ||
			authentication.Password == "" || len(authentication.Password) > 1024 {
			return application.CreateConnectionRequest{}, errInvalidEdit
		}
		request.Authentication = application.CreateAuthentication{
			Kind:       application.CreateAuthenticationNewSharedPassword,
			Credential: authentication.Credential, Password: authentication.Password,
		}
	case string(application.CreateAuthenticationIdentityFile):
		var authentication api.CreateIdentityFileAuthentication
		if err := decodeConnectionAuthentication(wire.Authentication, &authentication); err != nil ||
			len(authentication.KeyId) != 32 {
			return application.CreateConnectionRequest{}, errInvalidEdit
		}
		request.Authentication = application.CreateAuthentication{
			Kind: application.CreateAuthenticationIdentityFile, KeyID: authentication.KeyId,
		}
	default:
		return application.CreateConnectionRequest{}, errInvalidEdit
	}
	return request, nil
}

// The generated union preserves its JSON bytes so it can dispatch by
// discriminator. Decode each selected branch once more with unknown fields
// forbidden; otherwise a misspelled password/key field would be silently
// discarded by encoding/json inside the generated union helper.
func decodeConnectionAuthentication(value api.CreateConnectionAuthentication, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errInvalidEdit
	}
	return nil
}

func connectionProblem(c *echo.Context, err error) error {
	var storageConflict *storage.ConflictError
	switch {
	case errors.Is(err, errInvalidBody), errors.Is(err, errInvalidEdit),
		errors.Is(err, application.ErrInvalidAlias),
		errors.Is(err, application.ErrInvalidConnectionUser),
		errors.Is(err, application.ErrUnknownCreateAuthentication),
		errors.Is(err, application.ErrUnquotableValue),
		errors.Is(err, platform.ErrUnsafeHostname), errors.Is(err, platform.ErrUnsafePort):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, application.ErrInvalidIdentityFile):
		return problem(c, http.StatusUnprocessableEntity, "identity_file_invalid")
	case errors.Is(err, application.ErrConnectionDestinationExists):
		return problem(c, http.StatusConflict, "connection_destination_exists")
	case errors.Is(err, secret.ErrUnknownCredential):
		return problem(c, http.StatusNotFound, "unknown_credential")
	case errors.Is(err, secret.ErrLocked), errors.Is(err, secret.ErrNoVault),
		errors.Is(err, secret.ErrEmptySecret), errors.Is(err, secret.ErrUnsafeName):
		return passwordProblem(c, err)
	case errors.As(err, &storageConflict):
		return problem(c, http.StatusConflict, "vault_conflict")
	default:
		return serviceProblem(c, err)
	}
}
