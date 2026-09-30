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

	"sshc/internal/randomid"
	"sshc/internal/redact"
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
	var snippet Snippet
	err := s.mutate(func(library *Library) error {
		if len(library.Snippets) >= MaxSnippets {
			return ErrTooManySnippets
		}
		id, err := s.newUniqueSnippetID(*library)
		if err != nil {
			return err
		}
		moment := s.now().UTC()
		snippet = Snippet{
			ID: id, Name: draft.Name, Description: draft.Description, Command: draft.Command,
			Variables: cloneVariables(draft.Variables), CreatedAt: moment, UpdatedAt: moment,
		}
		library.Snippets = append(library.Snippets, snippet)
		return nil
	})
	if err != nil {
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
	var updated Snippet
	err := s.mutate(func(library *Library) error {
		index := snippetIndex(*library, id)
		if index < 0 {
			return ErrUnknownSnippet
		}
		updated = library.Snippets[index]
		updated.Name = draft.Name
		updated.Description = draft.Description
		updated.Command = draft.Command
		updated.Variables = cloneVariables(draft.Variables)
		updated.UpdatedAt = s.now().UTC()
		if updated.UpdatedAt.Before(updated.CreatedAt) {
			updated.UpdatedAt = updated.CreatedAt
		}
		// Existing startup bindings must remain executable after an edit.
		for _, startup := range library.Startup {
			if startup.SnippetID == id {
				if err := validateStartup(updated, startup.Inputs); err != nil {
					return err
				}
			}
		}
		library.Snippets[index] = updated
		return nil
	})
	if err != nil {
		return Snippet{}, err
	}
	return cloneSnippet(updated), nil
}

func (s *Service) Delete(id string) error {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	return s.mutate(func(library *Library) error {
		index := snippetIndex(*library, id)
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
		return nil
	})
}

// Startup は、割り当ての一覧と、それぞれを接続時に送れるかを返す。
func (s *Service) Startup() ([]StartupAssignment, error) {
	s.mutation.Lock()
	library, err := s.load()
	s.mutation.Unlock()
	if err != nil {
		return nil, err
	}
	assignments := make([]StartupAssignment, len(library.Startup))
	for index, startup := range library.Startup {
		assignments[index] = StartupAssignment{
			Alias:     startup.Alias,
			SnippetID: startup.SnippetID,
			Stale:     s.verifyStartupDestination(startup) != nil,
		}
	}
	return assignments, nil
}

// verifyStartupDestination は、割り当てた時の接続先と今の接続先が同じかを確かめる。
// 解決できない alias と、binding を持たない割り当ては、同じと言えないので送らない。
func (s *Service) verifyStartupDestination(startup Startup) error {
	if startup.Binding == "" {
		return ErrStartupDestinationChanged
	}
	if s.resolve == nil {
		return ErrNoResolver
	}
	resolution, err := s.resolve(startup.Alias)
	if err != nil {
		return err
	}
	if resolution.Binding != startup.Binding {
		return ErrStartupDestinationChanged
	}
	return nil
}

// SetStartup assigns a snippet to an alias. An empty snippet ID removes the
// assignment. Resolving the alias prevents a stale or pattern-only Host entry
// from becoming connection automation.
func (s *Service) SetStartup(alias, snippetID string, inputs map[string]string) error {
	if err := validate.Alias(alias); err != nil {
		return ErrInvalidTarget
	}
	binding := ""
	if snippetID != "" {
		if s.resolve == nil {
			return ErrNoResolver
		}
		resolution, err := s.resolve(alias)
		if err != nil {
			return err
		}
		binding = resolution.Binding
	}
	s.mutation.Lock()
	defer s.mutation.Unlock()
	return s.mutate(func(library *Library) error {
		filtered := library.Startup[:0]
		for _, startup := range library.Startup {
			if startup.Alias != alias {
				filtered = append(filtered, startup)
			}
		}
		library.Startup = filtered
		if snippetID == "" {
			return nil
		}
		index := snippetIndex(*library, snippetID)
		if index < 0 {
			return ErrUnknownSnippet
		}
		if err := validateStartup(library.Snippets[index], inputs); err != nil {
			return err
		}
		if len(library.Startup) >= MaxStartupBindings {
			return ErrTooManyStartupBindings
		}
		library.Startup = append(library.Startup, Startup{
			Alias: alias, SnippetID: snippetID, Inputs: cloneInputs(inputs), Binding: binding,
		})
		return nil
	})
}

func (s *Service) Preview(request PreviewRequest) (Preview, error) {
	preview, _, err := s.plan(request)
	return preview, err
}

// PrepareTerminalCommand expands one command without resolving an SSH alias.
// A terminal broadcast writes into a live PTY and therefore must preserve that
// session's cwd and shell state. Resolving the alias here would recreate the
// detached-command behaviour this path is meant to avoid.
func (s *Service) PrepareTerminalCommand(request CommandRequest) (PreparedCommand, error) {
	source, expanded, _, err := s.planSource(PreviewRequest{
		SnippetID: request.SnippetID,
		Command:   request.Command,
		Inputs:    request.Inputs,
	})
	if err != nil {
		return PreparedCommand{}, err
	}
	return prepareTerminalCommand(source, expanded)
}

func prepareTerminalCommand(source planSource, expanded expansion) (PreparedCommand, error) {
	publicEvidence, err := terminalEvidence(source, expanded.display)
	if err != nil {
		return PreparedCommand{}, err
	}
	actionEvidence, err := terminalEvidence(source, expanded.command)
	if err != nil {
		return PreparedCommand{}, err
	}
	return PreparedCommand{
		SnippetID:      source.snippetID,
		Command:        expanded.command,
		Display:        expanded.display,
		Evidence:       publicEvidence,
		ActionEvidence: actionEvidence,
	}, nil
}

func terminalEvidence(source planSource, command string) (string, error) {
	payload := struct {
		Kind      string    `json:"kind"`
		SnippetID string    `json:"snippetId,omitempty"`
		UpdatedAt time.Time `json:"updatedAt,omitempty"`
		Command   string    `json:"command"`
	}{Kind: source.kind, SnippetID: source.snippetID, UpdatedAt: source.updatedAt, Command: command}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// PrepareStartupCommand returns the executable command for the terminal
// injector. Public startup previews stay redacted; this internal path is the
// only one that may hand the expanded value to the PTY writer.
//
// 接続のたびに alias を解決し直し、割り当てた時の接続先と違えば送らない。入力値には
// シークレットが入るので、別の接続先のシェルへ打ち込まないためである。
func (s *Service) PrepareStartupCommand(alias string) (PreparedCommand, error) {
	startup, library, err := s.startupFor(alias)
	if err != nil {
		return PreparedCommand{}, err
	}
	if err := s.verifyStartupDestination(startup); err != nil {
		return PreparedCommand{}, err
	}
	source, expanded, _, err := planSourceFromLibrary(library, startup.SnippetID, startup.Inputs)
	if err != nil {
		return PreparedCommand{}, err
	}
	return prepareTerminalCommand(source, expanded)
}

func (s *Service) startupFor(alias string) (Startup, Library, error) {
	s.mutation.Lock()
	defer s.mutation.Unlock()
	library, err := s.load()
	if err != nil {
		return Startup{}, Library{}, err
	}
	for _, startup := range library.Startup {
		if startup.Alias == alias {
			return startup, library, nil
		}
	}
	return Startup{}, Library{}, ErrNoStartup
}

type plannedTarget struct {
	targetID string
	target   Target
	command  string
	secrets  []string
	run      RunFunc
}

type planSource struct {
	kind      string
	snippetID string
	updatedAt time.Time
	command   string
}

func (s *Service) plan(request PreviewRequest) (Preview, []plannedTarget, error) {
	source, expanded, secrets, err := s.planSource(request)
	if err != nil {
		return Preview{}, nil, err
	}
	return s.planExpanded(request, source, expanded, secrets)
}

func (s *Service) planExpanded(request PreviewRequest, source planSource, expanded expansion, secrets []string) (Preview, []plannedTarget, error) {
	if s.resolve == nil {
		return Preview{}, nil, ErrNoResolver
	}
	requested, err := normaliseTargets(request)
	if err != nil {
		return Preview{}, nil, ErrInvalidTarget
	}
	planned := make([]plannedTarget, 0, len(requested))
	public := make([]TargetPreview, 0, len(requested))
	for _, requestedTarget := range requested {
		alias := requestedTarget.Alias
		if err := validate.Alias(alias); err != nil {
			return Preview{}, nil, ErrInvalidTarget
		}
		resolution, err := s.resolve(alias)
		if err != nil {
			return Preview{}, nil, err
		}
		target := resolution.Target
		if target.Alias == "" {
			target.Alias = alias
		}
		planned = append(planned, plannedTarget{
			targetID: requestedTarget.TargetID, target: target, command: expanded.command, secrets: append([]string(nil), secrets...),
			run: resolution.Run,
		})
		public = append(public, TargetPreview{TargetID: requestedTarget.TargetID, Target: target, Command: expanded.display})
	}
	publicEvidence, err := planEvidence(source, planned, false)
	if err != nil {
		return Preview{}, nil, err
	}
	actionEvidence, err := planEvidence(source, planned, true)
	if err != nil {
		return Preview{}, nil, err
	}
	return Preview{
		Evidence: publicEvidence, ActionEvidence: actionEvidence,
		SnippetID: source.snippetID, Targets: public,
	}, planned, nil
}

func (s *Service) planSource(request PreviewRequest) (planSource, expansion, []string, error) {
	if (request.SnippetID == "") == (request.Command == "") {
		return planSource{}, expansion{}, nil, ErrInvalidSnippet
	}
	if request.Command != "" {
		if len(request.Command) > MaxCommandBytes || strings.IndexByte(request.Command, 0) >= 0 {
			return planSource{}, expansion{}, nil, ErrInvalidSnippet
		}
		if len(request.Inputs) > 0 {
			return planSource{}, expansion{}, nil, ErrUnknownVariable
		}
		expanded := expansion{command: request.Command, display: request.Command}
		return planSource{kind: "command", command: request.Command}, expanded, nil, nil
	}
	s.mutation.Lock()
	library, err := s.load()
	s.mutation.Unlock()
	if err != nil {
		return planSource{}, expansion{}, nil, err
	}
	return planSourceFromLibrary(library, request.SnippetID, request.Inputs)
}

func planSourceFromLibrary(library Library, snippetID string, inputs map[string]string) (planSource, expansion, []string, error) {
	index := snippetIndex(library, snippetID)
	if index < 0 {
		return planSource{}, expansion{}, nil, ErrUnknownSnippet
	}
	snippet := library.Snippets[index]
	expanded, err := expand(snippet.Command, snippet.Variables, inputs)
	if err != nil {
		return planSource{}, expansion{}, nil, err
	}
	return planSource{kind: "snippet", snippetID: snippet.ID, updatedAt: snippet.UpdatedAt}, expanded, secretValues(snippet.Variables, inputs), nil
}

func normaliseTargets(request PreviewRequest) ([]RequestedTarget, error) {
	if len(request.Aliases) > 0 && len(request.Targets) > 0 {
		return nil, ErrInvalidTarget
	}
	requested := request.Targets
	if len(request.Aliases) > 0 {
		requested = make([]RequestedTarget, len(request.Aliases))
		for index, alias := range request.Aliases {
			requested[index] = RequestedTarget{TargetID: alias, Alias: alias}
		}
	}
	if len(requested) == 0 || len(requested) > MaxTargets {
		return nil, ErrInvalidTarget
	}
	seen := make(map[string]bool, len(requested))
	for _, target := range requested {
		if target.TargetID == "" || len(target.TargetID) > MaxTargetIDBytes || strings.TrimSpace(target.TargetID) != target.TargetID || strings.IndexByte(target.TargetID, 0) >= 0 {
			return nil, ErrInvalidTarget
		}
		if seen[target.TargetID] {
			return nil, ErrDuplicateTarget
		}
		seen[target.TargetID] = true
	}
	return requested, nil
}

func planEvidence(source planSource, planned []plannedTarget, actual bool) (string, error) {
	type evidenceTarget struct {
		TargetID string `json:"targetId"`
		Target   Target `json:"target"`
		Command  string `json:"command"`
	}
	payload := struct {
		Kind      string           `json:"kind"`
		SnippetID string           `json:"snippetId,omitempty"`
		UpdatedAt time.Time        `json:"updatedAt,omitempty"`
		Command   string           `json:"command,omitempty"`
		Targets   []evidenceTarget `json:"targets"`
	}{Kind: source.kind, SnippetID: source.snippetID, UpdatedAt: source.updatedAt, Command: source.command}
	for _, target := range planned {
		command := redact.Values(target.command, target.secrets, secretRedaction)
		if actual {
			command = target.command
		}
		payload.Targets = append(payload.Targets, evidenceTarget{TargetID: target.targetID, Target: target.target, Command: command})
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

// newUnusedID は、inUse が使用中と答えない識別子を作る。スニペットとジョブの
// 識別子を並行に作っても、同じ乱数源を同時に読まない。
func (s *Service) newUnusedID(inUse func(id string) (bool, error)) (string, error) {
	s.ids.Lock()
	defer s.ids.Unlock()
	return randomid.UnusedID(s.random, inUse)
}

func (s *Service) newUniqueSnippetID(library Library) (string, error) {
	return s.newUnusedID(func(id string) (bool, error) { return snippetIndex(library, id) >= 0, nil })
}

func (s *Service) load() (Library, error) {
	if s.repository == nil {
		return Library{}, ErrNoRepository
	}
	return s.repository.Load()
}

func (s *Service) mutate(mutation func(*Library) error) error {
	if s.repository == nil {
		return ErrNoRepository
	}
	return s.repository.Mutate(mutation)
}

func fixedProblem(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		// 利用者が取り消していないのに、時間の上限で止まった。取り消しとは分ける。
		return "timed_out"
	default:
		return "run_failed"
	}
}
