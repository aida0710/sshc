package snippets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sshc/internal/storage"
	"sshc/internal/validate"
)

const temporaryName = ".snippets-"

type document struct {
	SchemaVersion int       `json:"schemaVersion"`
	Snippets      []Snippet `json:"snippets"`
	Startup       []Startup `json:"startup,omitempty"`
}

// Repository is the persistence seam used by Service.
type Repository interface {
	Load() (Library, error)
	Save(Library) error
}

// Store writes one private, atomically replaced document under ~/.ssh/sshc.
// It serialises filesystem access inside one engine; the engine lock prevents a
// second sshc process from concurrently editing the same workspace.
type Store struct {
	workspace *storage.Workspace
	mutex     sync.Mutex
}

func NewStore(workspace *storage.Workspace) *Store { return &Store{workspace: workspace} }

func (s *Store) Path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(PathRelative))
}

func (s *Store) Load() (Library, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.load()
}

func (s *Store) load() (Library, error) {
	contents, err := s.workspace.FileSystem().ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return Library{Snippets: []Snippet{}, Startup: []Startup{}}, nil
	}
	if err != nil {
		return Library{}, err
	}
	var stored document
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return Library{}, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Library{}, ErrInvalidDocument
	}
	if stored.SchemaVersion > SchemaVersion {
		return Library{}, ErrUnsupportedVersion
	}
	if stored.SchemaVersion != SchemaVersion {
		return Library{}, ErrInvalidDocument
	}
	library := Library{Snippets: stored.Snippets, Startup: stored.Startup}
	if err := validateLibrary(library); err != nil {
		return Library{}, fmt.Errorf("%w: %v", ErrInvalidDocument, err)
	}
	return cloneLibrary(library), nil
}

func (s *Store) Save(library Library) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := validateLibrary(library); err != nil {
		return err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	library = cloneLibrary(library)
	sort.Slice(library.Snippets, func(i, j int) bool { return library.Snippets[i].ID < library.Snippets[j].ID })
	sort.Slice(library.Startup, func(i, j int) bool { return library.Startup[i].Alias < library.Startup[j].Alias })
	contents, err := json.MarshalIndent(document{
		SchemaVersion: SchemaVersion, Snippets: library.Snippets, Startup: library.Startup,
	}, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	temporary, err := s.workspace.FileSystem().WriteTemp(
		s.workspace.StateDir(), temporaryName, storage.FilePermission, contents,
	)
	if err != nil {
		return err
	}
	defer func() { _ = s.workspace.FileSystem().Remove(temporary) }()
	if err := s.workspace.FileSystem().Rename(temporary, s.Path()); err != nil {
		return err
	}
	return s.workspace.FileSystem().SyncDir(s.workspace.StateDir())
}

func validateLibrary(library Library) error {
	if len(library.Snippets) > MaxSnippets || len(library.Startup) > MaxTargets {
		return ErrInvalidDocument
	}
	byID := make(map[string]Snippet, len(library.Snippets))
	for _, snippet := range library.Snippets {
		if err := validateSnippet(snippet); err != nil {
			return err
		}
		if _, exists := byID[snippet.ID]; exists {
			return ErrDuplicateSnippet
		}
		byID[snippet.ID] = snippet
	}
	aliases := make(map[string]bool, len(library.Startup))
	for _, startup := range library.Startup {
		if err := validate.Alias(startup.Alias); err != nil || len(startup.Alias) > 255 {
			return ErrInvalidTarget
		}
		if aliases[startup.Alias] {
			return ErrDuplicateTarget
		}
		aliases[startup.Alias] = true
		snippet, exists := byID[startup.SnippetID]
		if !exists {
			return ErrUnknownSnippet
		}
		if err := validateStartup(snippet, startup.Inputs); err != nil {
			return err
		}
	}
	return nil
}

func validateSnippet(snippet Snippet) error {
	if !snippetIDPattern.MatchString(snippet.ID) || strings.TrimSpace(snippet.Name) != snippet.Name || snippet.Name == "" ||
		len(snippet.Name) > MaxNameBytes || len(snippet.Description) > MaxDescriptionBytes || strings.IndexByte(snippet.Description, 0) >= 0 ||
		snippet.CreatedAt.IsZero() || snippet.UpdatedAt.IsZero() || snippet.UpdatedAt.Before(snippet.CreatedAt) || len(snippet.Variables) > MaxVariables {
		return ErrInvalidSnippet
	}
	_, err := expand(snippet.Command, snippet.Variables, defaultsForValidation(snippet.Variables))
	if errors.Is(err, ErrMissingVariable) {
		// Required inputs are expected at execution time. Validate the template and
		// definitions again with harmless type-correct placeholders.
		_, err = expand(snippet.Command, snippet.Variables, validationInputs(snippet.Variables))
	}
	return err
}

func validateDraft(draft Draft) error {
	moment := timeForValidation
	return validateSnippet(Snippet{
		ID: strings.Repeat("0", 32), Name: draft.Name, Description: draft.Description,
		Command: draft.Command, Variables: draft.Variables, CreatedAt: moment, UpdatedAt: moment,
	})
}

var timeForValidation = time.Unix(1, 0).UTC()

func defaultsForValidation(variables []Variable) map[string]string {
	inputs := make(map[string]string)
	for _, variable := range variables {
		if variable.Default != nil {
			inputs[variable.Name] = *variable.Default
		}
	}
	return inputs
}

func validationInputs(variables []Variable) map[string]string {
	inputs := make(map[string]string, len(variables))
	for _, variable := range variables {
		switch variable.Type {
		case VariableInteger:
			inputs[variable.Name] = "0"
		case VariableBoolean:
			inputs[variable.Name] = "false"
		default:
			inputs[variable.Name] = "value"
		}
	}
	return inputs
}

func validateStartup(snippet Snippet, inputs map[string]string) error {
	for _, variable := range snippet.Variables {
		if variable.Type == VariableSecret {
			return ErrSecretStartup
		}
	}
	_, err := expand(snippet.Command, snippet.Variables, inputs)
	return err
}

func cloneLibrary(library Library) Library {
	cloned := Library{
		Snippets: make([]Snippet, len(library.Snippets)),
		Startup:  make([]Startup, len(library.Startup)),
	}
	for index, snippet := range library.Snippets {
		cloned.Snippets[index] = cloneSnippet(snippet)
	}
	for index, startup := range library.Startup {
		cloned.Startup[index] = Startup{
			Alias: startup.Alias, SnippetID: startup.SnippetID, Inputs: cloneInputs(startup.Inputs),
		}
	}
	return cloned
}

func cloneSnippet(snippet Snippet) Snippet {
	cloned := snippet
	cloned.Variables = append([]Variable(nil), snippet.Variables...)
	for index := range cloned.Variables {
		if snippet.Variables[index].Default != nil {
			value := *snippet.Variables[index].Default
			cloned.Variables[index].Default = &value
		}
	}
	return cloned
}

func cloneInputs(inputs map[string]string) map[string]string {
	if len(inputs) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(inputs))
	for name, value := range inputs {
		cloned[name] = value
	}
	return cloned
}
