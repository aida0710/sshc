package application

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"unicode"

	"sshc/internal/config"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

var (
	ErrInvalidConnectionUser       = errors.New("user contains whitespace or control characters")
	ErrInvalidIdentityFile         = errors.New("identity file must name an inventoried private key")
	ErrConnectionDestinationExists = errors.New("the connection destination file already exists")
	ErrUnknownCreateAuthentication = errors.New("unknown connection authentication kind")
)

// CreateAuthenticationKind identifies the one source of authentication a new
// connection uses. Passwords are written only to the encrypted vault; private
// keys are represented in ssh_config by an IdentityFile directive.
type CreateAuthenticationKind string

const (
	CreateAuthenticationDedicatedPassword CreateAuthenticationKind = "dedicated_password"
	CreateAuthenticationSavedPassword     CreateAuthenticationKind = "saved_password"
	CreateAuthenticationNewSharedPassword CreateAuthenticationKind = "new_shared_password"
	CreateAuthenticationIdentityFile      CreateAuthenticationKind = "identity_file"
)

type CreateAuthentication struct {
	Kind       CreateAuthenticationKind `json:"kind"`
	Password   string                   `json:"password,omitempty"`
	Credential string                   `json:"credential,omitempty"`
	KeyID      string                   `json:"keyId,omitempty"`
}

type CreateConnectionRequest struct {
	Alias          string               `json:"alias"`
	Group          string               `json:"group,omitempty"`
	HostName       string               `json:"hostName"`
	User           string               `json:"user,omitempty"`
	Port           *int                 `json:"port,omitempty"`
	Authentication CreateAuthentication `json:"authentication"`
}

type CreateConnectionResult struct {
	TransactionID string       `json:"transactionId"`
	Identity      HostIdentity `json:"identity"`
	Preview       SavePreview  `json:"preview"`
}

// CreateConnection creates a complete Host block and, for password modes, the
// matching vault assignment in the same journalled storage transaction.
func (s *Service) CreateConnection(
	secrets *secret.Service,
	inventory *keys.Inventory,
	request CreateConnectionRequest,
) (CreateConnectionResult, error) {
	if err := validateCreateConnectionRequest(request); err != nil {
		return CreateConnectionResult{}, err
	}

	switch request.Authentication.Kind {
	case CreateAuthenticationIdentityFile:
		s.saveMutex.Lock()
		defer s.saveMutex.Unlock()
		prepared, identity, err := s.planCreateConnection(inventory, request)
		if err != nil {
			return CreateConnectionResult{}, err
		}
		result, err := s.commitCreatedConnection(prepared)
		if err != nil {
			return CreateConnectionResult{}, err
		}
		return CreateConnectionResult{
			TransactionID: result.ID,
			Identity:      identity,
			Preview:       prepared.preview,
		}, nil

	case CreateAuthenticationDedicatedPassword,
		CreateAuthenticationSavedPassword,
		CreateAuthenticationNewSharedPassword:
		if secrets == nil {
			return CreateConnectionResult{}, secret.ErrNoVault
		}
		exists, err := secrets.Exists()
		if err != nil {
			return CreateConnectionResult{}, err
		}
		if !exists {
			return CreateConnectionResult{}, secret.ErrNoVault
		}
		if !secrets.Unlocked() {
			return CreateConnectionResult{}, secret.ErrLocked
		}

		mutation := passwordMutationForCreate(request)
		var created CreateConnectionResult
		_, err = secrets.WithPasswordMutation(mutation, func(vaultChange storage.Change) (storage.Result, error) {
			s.saveMutex.Lock()
			defer s.saveMutex.Unlock()

			prepared, identity, planErr := s.planCreateConnection(inventory, request)
			if planErr != nil {
				return storage.Result{}, planErr
			}
			storageRequest := s.requestFor(prepared)
			storageRequest.Changes = append(storageRequest.Changes, vaultChange)
			result, commitErr := s.commitAtomicPlannedRequest(prepared, storageRequest)
			if commitErr != nil {
				return storage.Result{}, commitErr
			}
			created = CreateConnectionResult{
				TransactionID: result.ID,
				Identity:      identity,
				Preview:       prepared.preview,
			}
			return result, nil
		})
		if errors.Is(err, secret.ErrNoPasswordMutation) {
			s.saveMutex.Lock()
			defer s.saveMutex.Unlock()
			prepared, identity, planErr := s.planCreateConnection(inventory, request)
			if planErr != nil {
				return CreateConnectionResult{}, planErr
			}
			result, commitErr := s.commitCreatedConnection(prepared)
			if commitErr != nil {
				return CreateConnectionResult{}, commitErr
			}
			return CreateConnectionResult{
				TransactionID: result.ID,
				Identity:      identity,
				Preview:       prepared.preview,
			}, nil
		}
		if err != nil {
			return CreateConnectionResult{}, err
		}
		return created, nil
	default:
		return CreateConnectionResult{}, ErrUnknownCreateAuthentication
	}
}

func validateCreateConnectionRequest(request CreateConnectionRequest) error {
	if err := ValidateAlias(request.Alias); err != nil {
		return err
	}
	if err := platform.ValidateHostname(request.HostName); err != nil {
		return err
	}
	if err := validateConnectionUser(request.User); err != nil {
		return err
	}
	port := 22
	if request.Port != nil {
		port = *request.Port
	}
	if err := platform.ValidatePort(port); err != nil {
		return err
	}
	switch request.Authentication.Kind {
	case CreateAuthenticationDedicatedPassword,
		CreateAuthenticationSavedPassword,
		CreateAuthenticationNewSharedPassword,
		CreateAuthenticationIdentityFile:
		return nil
	default:
		return ErrUnknownCreateAuthentication
	}
}

func validateConnectionUser(user string) error {
	if user == "" {
		return nil
	}
	for _, character := range user {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return ErrInvalidConnectionUser
		}
	}
	return nil
}

func passwordMutationForCreate(request CreateConnectionRequest) secret.PasswordMutation {
	kind := secret.PasswordMutationDedicated
	switch request.Authentication.Kind {
	case CreateAuthenticationSavedPassword:
		kind = secret.PasswordMutationSaved
	case CreateAuthenticationNewSharedPassword:
		kind = secret.PasswordMutationNewShared
	}
	return secret.PasswordMutation{
		Kind:       kind,
		Alias:      request.Alias,
		Credential: request.Authentication.Credential,
		Password:   request.Authentication.Password,
	}
}

func (s *Service) planCreateConnection(
	inventory *keys.Inventory,
	request CreateConnectionRequest,
) (planned, HostIdentity, error) {
	graph, err := s.resolve()
	if err != nil {
		return planned{}, HostIdentity{}, err
	}
	if err := refuseTakenAlias(graph, "", request.Alias); err != nil {
		return planned{}, HostIdentity{}, err
	}

	port := 22
	if request.Port != nil {
		port = *request.Port
	}
	identityFile := ""
	if request.Authentication.Kind == CreateAuthenticationIdentityFile {
		identityFile, err = s.identityFileForCreate(inventory, request.Authentication.KeyID)
		if err != nil {
			return planned{}, HostIdentity{}, err
		}
	}
	block, err := createHostBlock(request.Alias, request.HostName, request.User, port, identityFile, "\n")
	if err != nil {
		return planned{}, HostIdentity{}, err
	}

	relative := entryFileName
	absolute := s.entryPath
	var previous []byte
	var exists bool
	var directories []string
	if request.Group != "" {
		if !slices.Contains(s.declaredGroups(graph), request.Group) {
			return planned{}, HostIdentity{}, ErrGroupNotDeclared
		}
		relative = path.Join(GroupDirectory(request.Group), GroupFileName(request.Alias))
		absolute = filepath.Join(s.workspace.Root(), filepath.FromSlash(relative))
		previous, exists, err = s.readFile(absolute)
		if err != nil {
			return planned{}, HostIdentity{}, err
		}
		if exists {
			return planned{}, HostIdentity{}, ErrConnectionDestinationExists
		}
		directories = []string{filepath.Dir(absolute)}
	} else {
		previous, exists, err = s.readFile(absolute)
		if err != nil {
			return planned{}, HostIdentity{}, err
		}
		file := config.Parse(previous)
		block, err = createHostBlock(request.Alias, request.HostName, request.User, port, identityFile, dominantEnding(file))
		if err != nil {
			return planned{}, HostIdentity{}, err
		}
		AppendHostBlock(file, block.Lines)
		block = file
	}
	updated := block.Render()

	precondition := storage.Precondition{}
	if exists {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(previous)}
	}
	cleaned := filepath.Clean(absolute)
	prepared := planned{
		operation:   "connection.create",
		changes:     []storage.Change{{Path: absolute, Contents: updated, Precondition: precondition}},
		directories: directories,
		base:        map[string][]byte{cleaned: previous},
		baseline:    diagnosticBaseline(graph),
		preview: SavePreview{
			Operation: "connection.create",
			Diffs:     []FileDiff{BuildFileDiff(relative, diskOrNil(previous, exists), updated)},
		},
	}

	after, err := s.resolveWith(map[string][]byte{cleaned: updated})
	if err != nil {
		return planned{}, HostIdentity{}, err
	}
	prepared.preview.Effective = []EffectiveDiff{DiffEffective(
		ComputeEffective(graph, s.workspace.Root(), request.Alias, s.localFacts()),
		ComputeEffective(after, s.workspace.Root(), request.Alias, s.localFacts()),
	)}
	return prepared, HostIdentity{Path: relative, Alias: request.Alias}, nil
}

func createHostBlock(alias, hostName, user string, port int, identityFile, ending string) (*config.File, error) {
	lines := make([]config.Line, 0, 5)
	appendLine := func(indent, keyword string, values ...string) error {
		line, err := buildLine(indent, keyword, values, ending)
		if err != nil {
			return err
		}
		lines = append(lines, line)
		return nil
	}
	if err := appendLine("", "Host", alias); err != nil {
		return nil, err
	}
	if err := appendLine("\t", "HostName", hostName); err != nil {
		return nil, err
	}
	if user != "" {
		if err := appendLine("\t", "User", user); err != nil {
			return nil, err
		}
	}
	if err := appendLine("\t", "Port", strconv.Itoa(port)); err != nil {
		return nil, err
	}
	if identityFile != "" {
		if err := appendLine("\t", "IdentityFile", identityFile); err != nil {
			return nil, err
		}
	}
	return &config.File{Lines: lines}, nil
}

func (s *Service) identityFileForCreate(inventory *keys.Inventory, keyID string) (string, error) {
	if inventory == nil {
		return "", ErrInvalidIdentityFile
	}
	item, ok := inventory.Find(keyID)
	if !ok || item.Kind != keys.KindPrivateKey {
		return "", ErrInvalidIdentityFile
	}
	relative := filepath.Clean(filepath.FromSlash(item.RelativePath))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." ||
		len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
		return "", ErrInvalidIdentityFile
	}
	absolute := filepath.Join(s.workspace.Root(), relative)
	if _, err := s.workspace.ResolveForWrite(absolute); err != nil {
		return "", ErrInvalidIdentityFile
	}
	info, err := s.workspace.FileSystem().Lstat(absolute)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", ErrInvalidIdentityFile
	}
	return "~/.ssh/" + filepath.ToSlash(relative), nil
}

func (s *Service) commitCreatedConnection(prepared planned) (storage.Result, error) {
	return s.commitPlannedRequest(prepared, s.requestFor(prepared))
}
