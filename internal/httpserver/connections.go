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
	engine.PATCH("/api/v1/connections", handlers.Update)
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

func (h ConnectionHandlers) Update(c *echo.Context) error {
	var wire api.UpdateConnectionRequest
	if err := decodeBody(c, &wire); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	defer wipeBuffer(wire.Password)
	defer wipeBuffer(wire.KeyPassphrase)
	defer func() {
		for _, value := range []*json.RawMessage{wire.HostName, wire.User, wire.Port, wire.IdentityFile} {
			if value != nil {
				wipeBuffer(*value)
			}
		}
	}()
	request, needsInventory, err := updateConnectionRequestFromAPI(wire)
	if err != nil {
		return connectionProblem(c, err)
	}
	var inventory *keys.Inventory
	if needsInventory {
		if h.Keys == nil {
			return problem(c, http.StatusInternalServerError, "inventory_failed")
		}
		inventory, err = h.Keys.Inventory()
		if err != nil {
			return problem(c, http.StatusInternalServerError, "inventory_failed")
		}
	}
	result, err := h.Service.UpdateConnection(h.Secrets, inventory, request)
	if err != nil {
		return connectionProblem(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func updateConnectionRequestFromAPI(wire api.UpdateConnectionRequest) (application.UpdateConnectionRequest, bool, error) {
	if wire.Identity.Path == "" || len(wire.Identity.Path) > 1024 ||
		wire.Identity.Alias == "" || len(wire.Identity.Alias) > 64 || len(wire.Base) > 1<<20 {
		return application.UpdateConnectionRequest{}, false, errInvalidEdit
	}
	request := application.UpdateConnectionRequest{
		Identity: application.HostIdentity{Path: wire.Identity.Path, Alias: wire.Identity.Alias},
		Base:     wire.Base,
	}
	if wire.HostName != nil {
		change, err := decodeStringConnectionChange(*wire.HostName)
		if err != nil {
			return application.UpdateConnectionRequest{}, false, err
		}
		request.HostName = &change
	}
	if wire.User != nil {
		change, err := decodeStringConnectionChange(*wire.User)
		if err != nil {
			return application.UpdateConnectionRequest{}, false, err
		}
		request.User = &change
	}
	if wire.Port != nil {
		change, err := decodePortConnectionChange(*wire.Port)
		if err != nil {
			return application.UpdateConnectionRequest{}, false, err
		}
		request.Port = &change
	}
	needsInventory := false
	if wire.IdentityFile != nil {
		change, err := decodeIdentityFileConnectionChange(*wire.IdentityFile)
		if err != nil {
			return application.UpdateConnectionRequest{}, false, err
		}
		request.IdentityFile = &change
		needsInventory = change.Action == application.ConnectionChangeSet
	}
	password, err := decodeUpdateConnectionPassword(wire.Password)
	if err != nil {
		return application.UpdateConnectionRequest{}, false, err
	}
	request.Password = password
	keyPassphrase, err := decodeUpdateConnectionKeyPassphrase(wire.KeyPassphrase)
	if err != nil {
		return application.UpdateConnectionRequest{}, false, err
	}
	request.KeyPassphrase = keyPassphrase
	return request, needsInventory, nil
}

func taggedValue(value json.RawMessage, field string) (string, error) {
	var tagged map[string]json.RawMessage
	if err := json.Unmarshal(value, &tagged); err != nil {
		return "", errInvalidEdit
	}
	raw, ok := tagged[field]
	if !ok {
		return "", errInvalidEdit
	}
	var tag string
	if err := json.Unmarshal(raw, &tag); err != nil || tag == "" {
		return "", errInvalidEdit
	}
	return tag, nil
}

func decodeStringConnectionChange(value api.ConnectionStringChange) (application.ConnectionStringChange, error) {
	action, err := taggedValue(value, "action")
	if err != nil {
		return application.ConnectionStringChange{}, err
	}
	switch application.ConnectionChangeAction(action) {
	case application.ConnectionChangeSet:
		var set api.ConnectionStringSet
		if err := decodeConnectionAuthentication(value, &set); err != nil || len(set.Value) > 255 {
			return application.ConnectionStringChange{}, errInvalidEdit
		}
		return application.ConnectionStringChange{Action: application.ConnectionChangeSet, Value: set.Value}, nil
	case application.ConnectionChangeInherit:
		var inherit api.ConnectionInherit
		if err := decodeConnectionAuthentication(value, &inherit); err != nil {
			return application.ConnectionStringChange{}, errInvalidEdit
		}
		return application.ConnectionStringChange{Action: application.ConnectionChangeInherit}, nil
	default:
		return application.ConnectionStringChange{}, errInvalidEdit
	}
}

func decodePortConnectionChange(value api.ConnectionPortChange) (application.ConnectionPortChange, error) {
	action, err := taggedValue(value, "action")
	if err != nil {
		return application.ConnectionPortChange{}, err
	}
	switch application.ConnectionChangeAction(action) {
	case application.ConnectionChangeSet:
		var set api.ConnectionPortSet
		if err := decodeConnectionAuthentication(value, &set); err != nil {
			return application.ConnectionPortChange{}, errInvalidEdit
		}
		return application.ConnectionPortChange{Action: application.ConnectionChangeSet, Value: set.Value}, nil
	case application.ConnectionChangeInherit:
		var inherit api.ConnectionInherit
		if err := decodeConnectionAuthentication(value, &inherit); err != nil {
			return application.ConnectionPortChange{}, errInvalidEdit
		}
		return application.ConnectionPortChange{Action: application.ConnectionChangeInherit}, nil
	default:
		return application.ConnectionPortChange{}, errInvalidEdit
	}
}

func decodeIdentityFileConnectionChange(value api.ConnectionIdentityFileChange) (application.ConnectionIdentityFileChange, error) {
	action, err := taggedValue(value, "action")
	if err != nil {
		return application.ConnectionIdentityFileChange{}, err
	}
	switch application.ConnectionChangeAction(action) {
	case application.ConnectionChangeSet:
		var set api.ConnectionIdentityFileSet
		if err := decodeConnectionAuthentication(value, &set); err != nil || len(set.KeyId) != 32 {
			return application.ConnectionIdentityFileChange{}, errInvalidEdit
		}
		return application.ConnectionIdentityFileChange{
			Action: application.ConnectionChangeSet, KeyID: set.KeyId,
		}, nil
	case application.ConnectionChangeInherit:
		var inherit api.ConnectionInherit
		if err := decodeConnectionAuthentication(value, &inherit); err != nil {
			return application.ConnectionIdentityFileChange{}, errInvalidEdit
		}
		return application.ConnectionIdentityFileChange{Action: application.ConnectionChangeInherit}, nil
	default:
		return application.ConnectionIdentityFileChange{}, errInvalidEdit
	}
}

func decodeUpdateConnectionPassword(value api.UpdateConnectionPassword) (application.UpdateConnectionPassword, error) {
	kind, err := taggedValue(value, "kind")
	if err != nil {
		return application.UpdateConnectionPassword{}, err
	}
	switch application.UpdateConnectionPasswordKind(kind) {
	case application.UpdatePasswordUnchanged:
		var unchanged api.UpdatePasswordUnchanged
		if err := decodeConnectionAuthentication(value, &unchanged); err != nil {
			return application.UpdateConnectionPassword{}, errInvalidEdit
		}
		return application.UpdateConnectionPassword{Kind: application.UpdatePasswordUnchanged}, nil
	case application.UpdatePasswordDedicated:
		var password api.CreateDedicatedPasswordAuthentication
		if err := decodeConnectionAuthentication(value, &password); err != nil ||
			password.Password == "" || len(password.Password) > 1024 {
			return application.UpdateConnectionPassword{}, errInvalidEdit
		}
		return application.UpdateConnectionPassword{
			Kind: application.UpdatePasswordDedicated, Password: password.Password,
		}, nil
	case application.UpdatePasswordSaved:
		var password api.CreateSavedPasswordAuthentication
		if err := decodeConnectionAuthentication(value, &password); err != nil ||
			password.Credential == "" || len(password.Credential) > 128 {
			return application.UpdateConnectionPassword{}, errInvalidEdit
		}
		return application.UpdateConnectionPassword{
			Kind: application.UpdatePasswordSaved, Credential: password.Credential,
		}, nil
	case application.UpdatePasswordNewShared:
		var password api.CreateNewSharedPasswordAuthentication
		if err := decodeConnectionAuthentication(value, &password); err != nil ||
			password.Credential == "" || len(password.Credential) > 128 ||
			password.Password == "" || len(password.Password) > 1024 {
			return application.UpdateConnectionPassword{}, errInvalidEdit
		}
		return application.UpdateConnectionPassword{
			Kind:       application.UpdatePasswordNewShared,
			Credential: password.Credential, Password: password.Password,
		}, nil
	case application.UpdatePasswordRemove:
		var remove api.UpdatePasswordRemove
		if err := decodeConnectionAuthentication(value, &remove); err != nil {
			return application.UpdateConnectionPassword{}, errInvalidEdit
		}
		return application.UpdateConnectionPassword{Kind: application.UpdatePasswordRemove}, nil
	default:
		return application.UpdateConnectionPassword{}, errInvalidEdit
	}
}

func decodeUpdateConnectionKeyPassphrase(value api.UpdateConnectionKeyPassphrase) (application.UpdateConnectionKeyPassphrase, error) {
	kind, err := taggedValue(value, "kind")
	if err != nil {
		return application.UpdateConnectionKeyPassphrase{}, err
	}
	switch application.UpdateConnectionKeyPassphraseKind(kind) {
	case application.UpdateKeyPassphraseUnchanged:
		var unchanged api.ConnectionKeyPassphraseUnchanged
		if err := decodeConnectionAuthentication(value, &unchanged); err != nil {
			return application.UpdateConnectionKeyPassphrase{}, errInvalidEdit
		}
		return application.UpdateConnectionKeyPassphrase{Kind: application.UpdateKeyPassphraseUnchanged}, nil
	case application.UpdateKeyPassphraseSetDedicated:
		var dedicated api.ConnectionKeyPassphraseSetDedicated
		if err := decodeConnectionAuthentication(value, &dedicated); err != nil ||
			len(dedicated.KeyId) != 32 || dedicated.Passphrase == "" || len(dedicated.Passphrase) > 1024 {
			return application.UpdateConnectionKeyPassphrase{}, errInvalidEdit
		}
		return application.UpdateConnectionKeyPassphrase{
			Kind:       application.UpdateKeyPassphraseSetDedicated,
			KeyID:      dedicated.KeyId,
			Passphrase: dedicated.Passphrase,
		}, nil
	default:
		return application.UpdateConnectionKeyPassphrase{}, errInvalidEdit
	}
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
		errors.Is(err, application.ErrUnknownCreateAuthentication),
		errors.Is(err, application.ErrUnknownConnectionChange),
		errors.Is(err, application.ErrUnknownUpdatePassword),
		errors.Is(err, application.ErrUnknownUpdateKeyPhrase),
		errors.Is(err, application.ErrUnquotableValue):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, platform.ErrUnsafeHostname):
		return problem(c, http.StatusBadRequest, "connection_hostname_invalid")
	case errors.Is(err, application.ErrInvalidConnectionUser):
		return problem(c, http.StatusBadRequest, "connection_user_invalid")
	case errors.Is(err, platform.ErrUnsafePort):
		return problem(c, http.StatusBadRequest, "connection_port_invalid")
	case errors.Is(err, application.ErrNoConnectionUpdate):
		return problem(c, http.StatusBadRequest, "connection_no_change")
	case errors.Is(err, application.ErrComplexConnectionField):
		return problem(c, http.StatusUnprocessableEntity, "connection_field_complex")
	case errors.Is(err, application.ErrPasswordIneligible):
		return problem(c, http.StatusUnprocessableEntity, "password_ineligible")
	case errors.Is(err, application.ErrInvalidIdentityFile):
		return problem(c, http.StatusUnprocessableEntity, "identity_file_invalid")
	case errors.Is(err, keys.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.Is(err, keys.ErrKeyChanged):
		return problem(c, http.StatusConflict, "external_change")
	case errors.Is(err, keys.ErrUnknownKey), errors.Is(err, keys.ErrKeyNotEncrypted):
		return problem(c, http.StatusUnprocessableEntity, "identity_file_invalid")
	case errors.Is(err, application.ErrConnectionDestinationExists):
		return problem(c, http.StatusConflict, "connection_destination_exists")
	case errors.Is(err, secret.ErrUnknownCredential):
		return problem(c, http.StatusNotFound, "unknown_credential")
	case errors.Is(err, secret.ErrCredentialAlreadyExists):
		return problem(c, http.StatusConflict, "credential_already_exists")
	case errors.Is(err, secret.ErrNoPassword):
		return problem(c, http.StatusNotFound, "password_missing")
	case errors.Is(err, secret.ErrLocked), errors.Is(err, secret.ErrNoVault),
		errors.Is(err, secret.ErrEmptySecret), errors.Is(err, secret.ErrUnsafeName):
		return passwordProblem(c, err)
	case errors.As(err, &storageConflict):
		return problem(c, http.StatusConflict, "vault_conflict")
	default:
		return serviceProblem(c, err)
	}
}
