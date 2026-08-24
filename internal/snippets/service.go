package snippets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"sshc/internal/validate"
)

type Options struct {
	Repository Repository
	Resolve    ResolveFunc
	Now        func() time.Time
	Random     io.Reader
}

type Service struct {
	repository Repository
	resolve    ResolveFunc
	now        func() time.Time
	random     io.Reader

	mutation sync.Mutex
	ids      sync.Mutex
	jobs     sync.Mutex
	active   map[string]*jobState
}

func NewService(options Options) *Service {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	return &Service{
		repository: options.Repository, resolve: options.Resolve,
		now: options.Now, random: options.Random, active: make(map[string]*jobState),
	}
}

func (s *Service) List() ([]Snippet, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return nil, err
	}
	result := make([]Snippet, len(library.Snippets))
	for index, snippet := range library.Snippets {
		result[index] = cloneSnippet(snippet)
	}
	return result, nil
}

func (s *Service) Create(draft Draft) (Snippet, error) {
	if err := validateDraft(draft); err != nil {
		return Snippet{}, err
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return Snippet{}, err
	}
	if len(library.Snippets) >= MaxSnippets {
		return Snippet{}, ErrInvalidDocument
	}
	id, err := s.newUniqueSnippetID(library)
	if err != nil {
		return Snippet{}, err
	}
	moment := s.now().UTC()
	snippet := Snippet{
		ID: id, Name: draft.Name, Description: draft.Description, Command: draft.Command,
		Variables: append([]Variable(nil), draft.Variables...), CreatedAt: moment, UpdatedAt: moment,
	}
	library.Snippets = append(library.Snippets, snippet)
	if err := s.save(library); err != nil {
		return Snippet{}, err
	}
	return cloneSnippet(snippet), nil
}

func (s *Service) Update(id string, draft Draft) (Snippet, error) {
	if err := validateDraft(draft); err != nil {
		return Snippet{}, err
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return Snippet{}, err
	}
	index := snippetIndex(library, id)
	if index < 0 {
		return Snippet{}, ErrUnknownSnippet
	}
	updated := library.Snippets[index]
	updated.Name = draft.Name
	updated.Description = draft.Description
	updated.Command = draft.Command
	updated.Variables = append([]Variable(nil), draft.Variables...)
	updated.UpdatedAt = s.now().UTC()
	if updated.UpdatedAt.Before(updated.CreatedAt) {
		updated.UpdatedAt = updated.CreatedAt
	}
	// Existing startup bindings must remain executable after an edit. Refuse an
	// incompatible edit instead of silently disabling connection automation.
	for _, startup := range library.Startup {
		if startup.SnippetID == id {
			if err := validateStartup(updated, startup.Inputs); err != nil {
				return Snippet{}, err
			}
		}
	}
	library.Snippets[index] = updated
	if err := s.save(library); err != nil {
		return Snippet{}, err
	}
	return cloneSnippet(updated), nil
}

func (s *Service) Delete(id string) error {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return err
	}
	index := snippetIndex(library, id)
	if index < 0 {
		return ErrUnknownSnippet
	}
	library.Snippets = append(library.Snippets[:index], library.Snippets[index+1:]...)
	startup := library.Startup[:0]
	for _, binding := range library.Startup {
		if binding.SnippetID != id {
			startup = append(startup, binding)
		}
	}
	library.Startup = startup
	return s.save(library)
}

func (s *Service) Startup() ([]Startup, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return nil, err
	}
	return cloneLibrary(library).Startup, nil
}

// SetStartup assigns a snippet to an alias. An empty snippet ID removes the
// assignment. Resolving the alias prevents a stale or pattern-only Host entry
// from becoming connection automation.
func (s *Service) SetStartup(alias, snippetID string, inputs map[string]string) error {
	if err := validate.Alias(alias); err != nil || len(alias) > 255 {
		return ErrInvalidTarget
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return err
	}
	filtered := library.Startup[:0]
	for _, startup := range library.Startup {
		if startup.Alias != alias {
			filtered = append(filtered, startup)
		}
	}
	library.Startup = filtered
	if snippetID == "" {
		return s.save(library)
	}
	if s.resolve == nil {
		return ErrNoResolver
	}
	if _, err := s.resolve(alias); err != nil {
		return err
	}
	index := snippetIndex(library, snippetID)
	if index < 0 {
		return ErrUnknownSnippet
	}
	if err := validateStartup(library.Snippets[index], inputs); err != nil {
		return err
	}
	library.Startup = append(library.Startup, Startup{
		Alias: alias, SnippetID: snippetID, Inputs: cloneInputs(inputs),
	})
	return s.save(library)
}

func (s *Service) Preview(request PreviewRequest) (Preview, error) {
	preview, _, err := s.plan(request)
	return preview, err
}

func (s *Service) PreviewStartup(alias string) (Preview, error) {
	s.mutation.Lock()
	library, err := s.load()
	s.mutation.Unlock()
	if err != nil {
		return Preview{}, err
	}
	for _, startup := range library.Startup {
		if startup.Alias == alias {
			return s.Preview(PreviewRequest{
				SnippetID: startup.SnippetID, Aliases: []string{alias}, Inputs: startup.Inputs,
			})
		}
	}
	return Preview{}, ErrNoStartup
}

type plannedTarget struct {
	target  Target
	command string
	secrets []string
	run     RunFunc
}

func (s *Service) plan(request PreviewRequest) (Preview, []plannedTarget, error) {
	if s.resolve == nil {
		return Preview{}, nil, ErrNoResolver
	}
	if len(request.Aliases) == 0 || len(request.Aliases) > MaxTargets {
		return Preview{}, nil, ErrInvalidTarget
	}
	s.mutation.Lock()
	library, err := s.load()
	s.mutation.Unlock()
	if err != nil {
		return Preview{}, nil, err
	}
	index := snippetIndex(library, request.SnippetID)
	if index < 0 {
		return Preview{}, nil, ErrUnknownSnippet
	}
	snippet := library.Snippets[index]
	expanded, err := expand(snippet.Command, snippet.Variables, request.Inputs)
	if err != nil {
		return Preview{}, nil, err
	}
	secrets := secretValues(snippet.Variables, request.Inputs)
	seen := make(map[string]bool, len(request.Aliases))
	planned := make([]plannedTarget, 0, len(request.Aliases))
	public := make([]TargetPreview, 0, len(request.Aliases))
	for _, alias := range request.Aliases {
		if err := validate.Alias(alias); err != nil || len(alias) > 255 {
			return Preview{}, nil, ErrInvalidTarget
		}
		if seen[alias] {
			return Preview{}, nil, ErrDuplicateTarget
		}
		seen[alias] = true
		resolution, err := s.resolve(alias)
		if err != nil {
			return Preview{}, nil, err
		}
		target := resolution.Target
		if target.Alias == "" {
			target.Alias = alias
		}
		planned = append(planned, plannedTarget{
			target: target, command: expanded.command, secrets: append([]string(nil), secrets...),
			run: resolution.Run,
		})
		public = append(public, TargetPreview{Target: target, Command: expanded.display})
	}
	evidence, err := planEvidence(snippet, planned)
	if err != nil {
		return Preview{}, nil, err
	}
	return Preview{Evidence: evidence, SnippetID: snippet.ID, Targets: public}, planned, nil
}

func planEvidence(snippet Snippet, planned []plannedTarget) (string, error) {
	type evidenceTarget struct {
		Target  Target `json:"target"`
		Command string `json:"command"`
	}
	payload := struct {
		SnippetID string           `json:"snippetId"`
		UpdatedAt time.Time        `json:"updatedAt"`
		Targets   []evidenceTarget `json:"targets"`
	}{SnippetID: snippet.ID, UpdatedAt: snippet.UpdatedAt}
	for _, target := range planned {
		payload.Targets = append(payload.Targets, evidenceTarget{Target: target.target, Command: target.command})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func evidenceMatches(want, got string) bool {
	if len(want) != sha256.Size*2 || len(got) != sha256.Size*2 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func secretValues(variables []Variable, inputs map[string]string) []string {
	var values []string
	for _, variable := range variables {
		if variable.Type != VariableSecret {
			continue
		}
		if value := inputs[variable.Name]; value != "" {
			values = append(values, value)
		}
	}
	return values
}

func snippetIndex(library Library, id string) int {
	for index, snippet := range library.Snippets {
		if snippet.ID == id {
			return index
		}
	}
	return -1
}

func (s *Service) newID() (string, error) {
	s.ids.Lock()
	defer s.ids.Unlock()
	raw := make([]byte, 16)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (s *Service) newUniqueSnippetID(library Library) (string, error) {
	for range 8 {
		id, err := s.newID()
		if err != nil {
			return "", err
		}
		if snippetIndex(library, id) < 0 {
			return id, nil
		}
	}
	return "", ErrDuplicateSnippet
}

func (s *Service) load() (Library, error) {
	if s.repository == nil {
		return Library{}, ErrNoRepository
	}
	return s.repository.Load()
}

func (s *Service) save(library Library) error {
	if s.repository == nil {
		return ErrNoRepository
	}
	return s.repository.Save(library)
}

func fixedProblem(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "cancelled"
	}
	return "run_failed"
}

func redact(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, secretRedaction)
		}
	}
	return value
}
