package application

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/keys"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/validate"
)

var (
	ErrNoConnectionUpdate      = errors.New("connection update makes no change")
	ErrComplexConnectionField  = errors.New("connection field has multiple direct directives")
	ErrUnknownConnectionChange = errors.New("unknown connection field change action")
	ErrUnknownUpdatePassword   = errors.New("unknown connection password update kind")
	ErrUnknownUpdateKeyPhrase  = errors.New("unknown connection key passphrase update kind")
	ErrPasswordIneligible      = errors.New("connection cannot use a stored password")
)

type KeyPassphraseVerifier interface {
	VerifyPassphrase(keyID string, passphrase []byte) (keys.PassphraseVerification, error)
	RevalidatePassphrase(verification keys.PassphraseVerification) error
}

// SetKeyPassphraseVerifier installs the key-vault boundary used by connection
// saves. Application wiring calls this once before the server begins serving.
func (s *Service) SetKeyPassphraseVerifier(verifier KeyPassphraseVerifier) {
	s.keyPassphrases = verifier
}

type ConnectionChangeAction string

const (
	ConnectionChangeSet     ConnectionChangeAction = "set"
	ConnectionChangeInherit ConnectionChangeAction = "inherit"
)

type ConnectionStringChange struct {
	Action ConnectionChangeAction `json:"action"`
	Value  string                 `json:"value,omitempty"`
}

type ConnectionPortChange struct {
	Action ConnectionChangeAction `json:"action"`
	Value  int                    `json:"value,omitempty"`
}

type ConnectionIdentityFileChange struct {
	Action ConnectionChangeAction `json:"action"`
	KeyID  string                 `json:"keyId,omitempty"`
}

type UpdateConnectionPasswordKind string

const (
	UpdatePasswordUnchanged UpdateConnectionPasswordKind = "unchanged"
	UpdatePasswordDedicated UpdateConnectionPasswordKind = "dedicated_password"
	UpdatePasswordSaved     UpdateConnectionPasswordKind = "saved_password"
	UpdatePasswordNewShared UpdateConnectionPasswordKind = "new_shared_password"
	UpdatePasswordRemove    UpdateConnectionPasswordKind = "remove"
)

type UpdateConnectionPassword struct {
	Kind       UpdateConnectionPasswordKind `json:"kind"`
	Password   string                       `json:"password,omitempty"`
	Credential string                       `json:"credential,omitempty"`
}

type UpdateConnectionKeyPassphraseKind string

const (
	UpdateKeyPassphraseUnchanged    UpdateConnectionKeyPassphraseKind = "unchanged"
	UpdateKeyPassphraseSetDedicated UpdateConnectionKeyPassphraseKind = "set_dedicated"
)

type UpdateConnectionKeyPassphrase struct {
	Kind       UpdateConnectionKeyPassphraseKind `json:"kind"`
	KeyID      string                            `json:"keyId,omitempty"`
	Passphrase string                            `json:"passphrase,omitempty"`
}

type UpdateConnectionRequest struct {
	Identity      HostIdentity                  `json:"identity"`
	Base          string                        `json:"base"`
	HostName      *ConnectionStringChange       `json:"hostName,omitempty"`
	User          *ConnectionStringChange       `json:"user,omitempty"`
	Port          *ConnectionPortChange         `json:"port,omitempty"`
	IdentityFile  *ConnectionIdentityFileChange `json:"identityFile,omitempty"`
	Password      UpdateConnectionPassword      `json:"password"`
	KeyPassphrase UpdateConnectionKeyPassphrase `json:"keyPassphrase"`
}

// UpdateConnection changes the small, stable connection form. The browser
// names semantic fields; line numbers are derived against the exact base file
// here so a sparse block can add a value and a direct value can return to
// inheritance without letting the client target another line.
func (s *Service) UpdateConnection(
	secrets *secret.Service,
	inventory *keys.Inventory,
	request UpdateConnectionRequest,
) (SaveResult, error) {
	passwordUnchanged := request.Password.Kind == "" || request.Password.Kind == UpdatePasswordUnchanged
	keyPassphraseUnchanged := request.KeyPassphrase.Kind == "" || request.KeyPassphrase.Kind == UpdateKeyPassphraseUnchanged
	if !keyPassphraseUnchanged && request.KeyPassphrase.Kind != UpdateKeyPassphraseSetDedicated {
		return SaveResult{}, ErrUnknownUpdateKeyPhrase
	}

	// Plan before inspecting policy so a stale base remains a conflict rather
	// than being masked by the authentication state of newer disk contents.
	s.saveMutex.Lock()
	prepared, changed, err := s.planConnectionUpdate(inventory, request)
	s.saveMutex.Unlock()
	if err != nil {
		return SaveResult{}, err
	}
	if prepared.explicitIdentityFile && !passwordUnchanged && request.Password.Kind != UpdatePasswordRemove {
		return SaveResult{}, ErrPasswordIneligible
	}
	if !prepared.explicitIdentityFile && !passwordUnchanged && request.Password.Kind != UpdatePasswordRemove &&
		prepared.passwordAuthenticationOff {
		return SaveResult{}, ErrPasswordIneligible
	}

	passwordCleanup := prepared.explicitIdentityFile && passwordUnchanged
	if passwordUnchanged && keyPassphraseUnchanged && !passwordCleanup {
		if !changed {
			return SaveResult{}, ErrNoConnectionUpdate
		}
		s.saveMutex.Lock()
		defer s.saveMutex.Unlock()
		prepared, changed, err = s.planConnectionUpdate(inventory, request)
		if err != nil {
			return SaveResult{}, err
		}
		if !changed {
			return SaveResult{}, ErrNoConnectionUpdate
		}
		result, err := s.commitPlannedRequest(prepared, s.requestFor(prepared))
		if err != nil {
			return SaveResult{}, err
		}
		return s.connectionUpdateResult(result, prepared), nil
	}
	if secrets == nil {
		return SaveResult{}, secret.ErrNoVault
	}
	mutation := secret.ConnectionSecretsMutation{}
	if passwordCleanup {
		mutation.Password = &secret.PasswordMutation{
			Kind: secret.PasswordMutationRemove, Alias: request.Identity.Alias,
		}
	} else if !passwordUnchanged {
		passwordMutation, err := passwordMutationForUpdate(request)
		if err != nil {
			return SaveResult{}, err
		}
		mutation.Password = &passwordMutation
	}
	var verification *keys.PassphraseVerification
	if !keyPassphraseUnchanged {
		if s.keyPassphrases == nil {
			return SaveResult{}, ErrInvalidIdentityFile
		}
		verified, err := s.keyPassphrases.VerifyPassphrase(
			request.KeyPassphrase.KeyID, []byte(request.KeyPassphrase.Passphrase),
		)
		if err != nil {
			return SaveResult{}, err
		}
		verification = &verified
		mutation.KeyPassphrase = &secret.KeyPassphraseMutation{
			RelativePath: verified.RelativePath,
			Passphrase:   request.KeyPassphrase.Passphrase,
		}
	}

	var updated SaveResult
	_, err = secrets.WithConnectionSecretsTransaction(mutation, func(vaultChange *storage.Change) (storage.Result, error) {
		s.saveMutex.Lock()
		defer s.saveMutex.Unlock()
		prepared, changed, planErr := s.planConnectionUpdate(inventory, request)
		if planErr != nil {
			return storage.Result{}, planErr
		}
		if prepared.explicitIdentityFile && !passwordUnchanged && request.Password.Kind != UpdatePasswordRemove {
			return storage.Result{}, ErrPasswordIneligible
		}
		if !prepared.explicitIdentityFile && !passwordUnchanged && request.Password.Kind != UpdatePasswordRemove &&
			prepared.passwordAuthenticationOff {
			return storage.Result{}, ErrPasswordIneligible
		}
		if validationErr := s.validateConnectionKeyPassphrase(prepared, request, verification); validationErr != nil {
			return storage.Result{}, validationErr
		}
		if vaultChange == nil && !changed {
			return storage.Result{}, ErrNoConnectionUpdate
		}
		storageRequest := s.requestFor(prepared)
		var result storage.Result
		var commitErr error
		if vaultChange == nil {
			result, commitErr = s.commitPlannedRequest(prepared, storageRequest)
		} else {
			storageRequest.Changes = append(storageRequest.Changes, *vaultChange)
			result, commitErr = s.commitAtomicPlannedRequest(prepared, storageRequest)
		}
		if commitErr != nil {
			return storage.Result{}, commitErr
		}
		updated = s.connectionUpdateResult(result, prepared)
		return result, nil
	})
	if err != nil {
		return SaveResult{}, err
	}
	return updated, nil
}

func (s *Service) validateConnectionKeyPassphrase(
	prepared planned,
	request UpdateConnectionRequest,
	verification *keys.PassphraseVerification,
) error {
	if verification == nil {
		return nil
	}
	if verification.KeyID != request.KeyPassphrase.KeyID {
		return ErrInvalidIdentityFile
	}
	absolute, err := AbsolutePath(s.workspace.Root(), request.Identity.Path)
	if err != nil {
		return err
	}
	contents := []byte(request.Base)
	for _, change := range prepared.changes {
		if filepath.Clean(change.Path) == filepath.Clean(absolute) {
			contents = change.Contents
			break
		}
	}
	file := config.Parse(contents)
	block, ok := FindHostBlock(file, request.Identity.Alias)
	if !ok {
		return ErrInvalidIdentityFile
	}
	identityFiles := make([]string, 0, 1)
	for index := block.Start; index < block.End && index < len(file.Lines); index++ {
		line := file.Lines[index]
		if line.Kind != config.LineDirective || !strings.EqualFold(line.Keyword, "IdentityFile") {
			continue
		}
		values := line.Values()
		if len(values) != 1 {
			return ErrInvalidIdentityFile
		}
		identityFiles = append(identityFiles, values[0])
	}
	want := "~/.ssh/" + filepath.ToSlash(verification.RelativePath)
	if len(identityFiles) != 1 || identityFiles[0] != want {
		return ErrInvalidIdentityFile
	}
	return s.keyPassphrases.RevalidatePassphrase(*verification)
}

func passwordMutationForUpdate(request UpdateConnectionRequest) (secret.PasswordMutation, error) {
	kind := secret.PasswordMutationKind("")
	switch request.Password.Kind {
	case UpdatePasswordDedicated:
		kind = secret.PasswordMutationDedicated
	case UpdatePasswordSaved:
		kind = secret.PasswordMutationSaved
	case UpdatePasswordNewShared:
		kind = secret.PasswordMutationNewShared
	case UpdatePasswordRemove:
		kind = secret.PasswordMutationRemove
	default:
		return secret.PasswordMutation{}, ErrUnknownUpdatePassword
	}
	return secret.PasswordMutation{
		Kind: kind, Alias: request.Identity.Alias,
		Credential: request.Password.Credential, Password: request.Password.Password,
	}, nil
}

func (s *Service) connectionUpdateResult(result storage.Result, prepared planned) SaveResult {
	vaultPath := filepath.Clean(filepath.Join(s.workspace.Root(), filepath.FromSlash(secret.WorkspacePath)))
	written := make([]string, 0, len(result.Written))
	for _, path := range result.Written {
		if filepath.Clean(path) == vaultPath {
			continue
		}
		written = append(written, s.displayPath(path))
	}
	return SaveResult{TransactionID: result.ID, Written: written, Preview: prepared.preview}
}

func (s *Service) planConnectionUpdate(inventory *keys.Inventory, request UpdateConnectionRequest) (planned, bool, error) {
	if err := ValidateAlias(request.Identity.Alias); err != nil {
		return planned{}, false, err
	}
	graph, err := s.resolve()
	if err != nil {
		return planned{}, false, err
	}
	absolute, err := AbsolutePath(s.workspace.Root(), request.Identity.Path)
	if err != nil {
		return planned{}, false, err
	}
	if _, err := s.workspace.ResolveForWrite(absolute); err != nil {
		return planned{}, false, err
	}
	node, reached := graph.Nodes[absolute]
	if !reached || node.File == nil {
		return planned{}, false, ErrHostNotFound
	}
	if !node.Editable {
		return planned{}, false, ErrNotEditable
	}
	if _, ok := FindHostBlock(node.File, request.Identity.Alias); !ok {
		return planned{}, false, ErrHostNotFound
	}
	base := []byte(request.Base)
	current := node.File.Render()
	if !bytes.Equal(base, current) {
		return planned{}, false, &ConflictError{Report: BuildConflictReport(
			request.Identity.Path, base, current, base,
		)}
	}
	file := config.Parse(base)
	block, ok := FindHostBlock(file, request.Identity.Alias)
	if !ok {
		return planned{}, false, ErrHostNotFound
	}

	edits := make([]FieldEdit, 0, 4)
	appendEdit := func(keyword string, action ConnectionChangeAction, values []string) error {
		edit, changed, editErr := connectionFieldEdit(file, block, keyword, action, values)
		if editErr != nil {
			return editErr
		}
		if changed {
			edits = append(edits, edit)
		}
		return nil
	}
	if request.HostName != nil {
		if request.HostName.Action == ConnectionChangeSet {
			if err := validate.Hostname(request.HostName.Value); err != nil {
				return planned{}, false, err
			}
		}
		if err := appendEdit("HostName", request.HostName.Action, []string{request.HostName.Value}); err != nil {
			return planned{}, false, err
		}
	}
	if request.User != nil {
		if request.User.Action == ConnectionChangeSet {
			if request.User.Value == "" {
				return planned{}, false, ErrInvalidConnectionUser
			}
			if err := validateConnectionUser(request.User.Value); err != nil {
				return planned{}, false, err
			}
		}
		if err := appendEdit("User", request.User.Action, []string{request.User.Value}); err != nil {
			return planned{}, false, err
		}
	}
	if request.Port != nil {
		if request.Port.Action == ConnectionChangeSet {
			if err := validate.Port(request.Port.Value); err != nil {
				return planned{}, false, err
			}
		}
		if err := appendEdit("Port", request.Port.Action, []string{strconv.Itoa(request.Port.Value)}); err != nil {
			return planned{}, false, err
		}
	}
	if request.IdentityFile != nil {
		value := ""
		if request.IdentityFile.Action == ConnectionChangeSet {
			value, err = s.identityFileForCreate(inventory, request.IdentityFile.KeyID)
			if err != nil {
				return planned{}, false, err
			}
		}
		if err := appendEdit("IdentityFile", request.IdentityFile.Action, []string{value}); err != nil {
			return planned{}, false, err
		}
	}

	if len(edits) == 0 {
		prepared := planned{
			operation: "connection.update", base: map[string][]byte{filepath.Clean(absolute): base},
			baseline: diagnosticBaseline(graph), preview: SavePreview{Operation: "connection.update", Diffs: []FileDiff{}},
		}
		_, prepared.explicitIdentityFile = directIdentityFile(file, block)
		_, prepared.passwordAuthenticationOff = passwordAuthenticationDisabled(
			effective.Resolve(graph, request.Identity.Alias, s.localFacts()))
		return prepared, false, s.verifyConnectionUpdateBase(absolute, request.Identity.Path, base, base)
	}
	if err := ApplyFieldEdits(file, block, edits); err != nil {
		return planned{}, false, err
	}
	updated := file.Render()
	if err := s.verifyConnectionUpdateBase(absolute, request.Identity.Path, base, updated); err != nil {
		return planned{}, false, err
	}
	prepared := planned{
		operation: "connection.update",
		changes: []storage.Change{{
			Path: absolute, Contents: updated,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(base)},
		}},
		base:     map[string][]byte{filepath.Clean(absolute): base},
		baseline: diagnosticBaseline(graph),
		preview: SavePreview{
			Operation: "connection.update",
			Diffs:     []FileDiff{BuildFileDiff(request.Identity.Path, base, updated)},
		},
	}
	after, err := s.resolveWith(map[string][]byte{filepath.Clean(absolute): updated})
	if err != nil {
		return planned{}, false, err
	}
	prepared.preview.Effective = []EffectiveDiff{DiffEffective(
		ComputeEffective(graph, s.workspace.Root(), request.Identity.Alias, s.localFacts()),
		ComputeEffective(after, s.workspace.Root(), request.Identity.Alias, s.localFacts()),
	)}
	updatedBlock, ok := FindHostBlock(file, request.Identity.Alias)
	if !ok {
		return planned{}, false, ErrHostNotFound
	}
	_, prepared.explicitIdentityFile = directIdentityFile(file, updatedBlock)
	_, prepared.passwordAuthenticationOff = passwordAuthenticationDisabled(
		effective.Resolve(after, request.Identity.Alias, s.localFacts()))
	return prepared, true, nil
}

func (s *Service) verifyConnectionUpdateBase(absolute, relative string, base, updated []byte) error {
	disk, exists, err := s.readFile(absolute)
	if err != nil {
		return err
	}
	if !exists || !bytes.Equal(base, disk) {
		return &ConflictError{Report: BuildConflictReport(relative, base, disk, updated)}
	}
	return nil
}

func connectionFieldEdit(
	file *config.File,
	block config.Block,
	keyword string,
	action ConnectionChangeAction,
	values []string,
) (FieldEdit, bool, error) {
	if action != ConnectionChangeSet && action != ConnectionChangeInherit {
		return FieldEdit{}, false, ErrUnknownConnectionChange
	}
	lines := make([]int, 0, 1)
	for index := block.Start; index < block.End && index < len(file.Lines); index++ {
		line := file.Lines[index]
		if line.Kind == config.LineDirective && strings.EqualFold(line.Keyword, keyword) {
			lines = append(lines, index+1)
		}
	}
	if len(lines) > 1 {
		return FieldEdit{}, false, ErrComplexConnectionField
	}
	if action == ConnectionChangeInherit {
		if len(lines) == 0 {
			return FieldEdit{}, false, nil
		}
		return FieldEdit{Action: ActionRemove, Line: lines[0]}, true, nil
	}
	if len(lines) == 0 {
		return FieldEdit{Action: ActionAdd, Keyword: keyword, Values: values}, true, nil
	}
	if slices.Equal(file.Lines[lines[0]-1].Values(), values) {
		return FieldEdit{}, false, nil
	}
	return FieldEdit{Action: ActionSet, Line: lines[0], Values: values}, true, nil
}
