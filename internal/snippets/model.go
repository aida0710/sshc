package snippets

import (
	"context"
	"errors"
	"time"
)

const (
	SchemaVersion         = 1
	PathRelative          = "sshc/snippets.json"
	MaxSnippets           = 256
	MaxVariables          = 32
	MaxTargets            = 64
	MaxConcurrency        = 8
	DefaultConcurrency    = 4
	MaxNameBytes          = 96
	MaxDescriptionBytes   = 2048
	MaxCommandBytes       = 64 << 10
	MaxVariableValueBytes = 16 << 10
	MaxResultBytes        = 256 << 10
	MaxRetainedJobs       = 128
)

var (
	ErrInvalidSnippet     = errors.New("snippet is invalid")
	ErrInvalidVariable    = errors.New("snippet variable is invalid")
	ErrUnknownVariable    = errors.New("input was supplied for an unknown variable")
	ErrMissingVariable    = errors.New("a required snippet variable is missing")
	ErrMalformedTemplate  = errors.New("snippet template contains a malformed placeholder")
	ErrUnknownSnippet     = errors.New("snippet does not exist")
	ErrDuplicateSnippet   = errors.New("snippet id already exists")
	ErrInvalidTarget      = errors.New("snippet target is invalid")
	ErrDuplicateTarget    = errors.New("snippet target appears more than once")
	ErrPreviewChanged     = errors.New("snippet preview changed before execution")
	ErrNoResolver         = errors.New("no snippet target resolver is available")
	ErrNoRunner           = errors.New("no snippet command runner is available")
	ErrNoRepository       = errors.New("no snippet repository is available")
	ErrInvalidDocument    = errors.New("snippets document is invalid")
	ErrUnsupportedVersion = errors.New("snippets were written by a newer version of sshc")
	ErrSecretStartup      = errors.New("startup snippets cannot persist secret variable values")
	ErrUnknownJob         = errors.New("snippet execution job does not exist")
	ErrJobFinished        = errors.New("snippet execution job has already finished")
	ErrTooManyJobs        = errors.New("too many snippet execution jobs are active")
	ErrNoStartup          = errors.New("host has no startup snippet")
)

type VariableType string

const (
	VariableString  VariableType = "string"
	VariableInteger VariableType = "integer"
	VariableBoolean VariableType = "boolean"
	VariableSecret  VariableType = "secret"
)

// Variable declares one {{name}} placeholder. Default is nil when no default
// exists; an explicit empty-string default is therefore distinguishable.
type Variable struct {
	Name        string       `json:"name"`
	Type        VariableType `json:"type"`
	Required    bool         `json:"required,omitempty"`
	Default     *string      `json:"default,omitempty"`
	Description string       `json:"description,omitempty"`
}

type Snippet struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Command     string     `json:"command"`
	Variables   []Variable `json:"variables"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// Draft is the caller-editable portion of a snippet. IDs and timestamps are
// assigned by Service.
type Draft struct {
	Name        string
	Description string
	Command     string
	Variables   []Variable
}

// Startup binds one host alias to one snippet. Inputs may contain only
// non-secret variables. They travel with the snippet configuration.
type Startup struct {
	Alias     string            `json:"alias"`
	SnippetID string            `json:"snippetId"`
	Inputs    map[string]string `json:"inputs,omitempty"`
}

type Library struct {
	Snippets []Snippet `json:"snippets"`
	Startup  []Startup `json:"startup,omitempty"`
}

type Target struct {
	Alias    string `json:"alias"`
	HostName string `json:"hostName"`
	User     string `json:"user"`
	Port     string `json:"port"`
	// Route is ordered from the first ProxyJump hop to the final destination.
	// It is both shown at confirmation time and included in preview evidence, so
	// a later ProxyCommand or authentication-route change invalidates the token.
	Route []RouteHop `json:"route,omitempty"`
}

type RouteHop struct {
	Alias             string   `json:"alias"`
	HostName          string   `json:"hostName"`
	User              string   `json:"user"`
	Port              string   `json:"port"`
	ProxyCommand      string   `json:"proxyCommand,omitempty"`
	StrictHostKey     string   `json:"strictHostKey"`
	Authentication    []string `json:"authentication"`
	IdentityFiles     []string `json:"identityFiles,omitempty"`
	IdentitiesOnly    bool     `json:"identitiesOnly,omitempty"`
	HostKeyAlgorithms []string `json:"hostKeyAlgorithms,omitempty"`
}

type CommandOutput struct {
	ExitCode  int
	Stdout    []byte
	Stderr    []byte
	Truncated bool
}

type RunFunc func(ctx context.Context, command string) (CommandOutput, error)

// Resolution binds the displayable target to the exact resolved SSH target
// that Run will use. The adapter should capture its native sshclient.Target in
// Run so configuration cannot change between evidence verification and launch.
type Resolution struct {
	Target Target
	Run    RunFunc
}

type ResolveFunc func(alias string) (Resolution, error)

// RequestedTarget gives one execution target a caller-stable identity. Alias
// resolves the SSH host; TargetID distinguishes multiple workspace panes that
// intentionally point at the same alias.
type RequestedTarget struct {
	TargetID string `json:"targetId"`
	Alias    string `json:"alias"`
}

type PreviewRequest struct {
	SnippetID string            `json:"snippetId,omitempty"`
	Command   string            `json:"command,omitempty"`
	Aliases   []string          `json:"aliases,omitempty"`
	Targets   []RequestedTarget `json:"targets,omitempty"`
	Inputs    map[string]string `json:"inputs"`
}

type TargetPreview struct {
	TargetID string `json:"targetId"`
	Target   Target `json:"target"`
	Command  string `json:"command"`
}

type Preview struct {
	Evidence  string          `json:"evidence"`
	SnippetID string          `json:"snippetId"`
	Targets   []TargetPreview `json:"targets"`
}

// ActionTarget binds the one-time confirmation token to the selected snippet
// or, for an ad-hoc command, to the exact preview evidence.
func (p Preview) ActionTarget() string {
	if p.SnippetID != "" {
		return p.SnippetID
	}
	return "command:" + p.Evidence
}

type ExecuteRequest struct {
	PreviewRequest
	Evidence    string `json:"evidence"`
	Concurrency int    `json:"concurrency"`
}

type TargetStatus string

const (
	TargetQueued    TargetStatus = "queued"
	TargetRunning   TargetStatus = "running"
	TargetSucceeded TargetStatus = "succeeded"
	TargetFailed    TargetStatus = "failed"
	TargetCancelled TargetStatus = "cancelled"
)

type TargetResult struct {
	TargetID  string       `json:"targetId"`
	Alias     string       `json:"alias"`
	Status    TargetStatus `json:"status"`
	ExitCode  int          `json:"exitCode,omitempty"`
	Stdout    string       `json:"stdout,omitempty"`
	Stderr    string       `json:"stderr,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
	Problem   string       `json:"problem,omitempty"`
}

type JobStatus string

const (
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobCancelled JobStatus = "cancelled"
)

type Job struct {
	ID         string         `json:"id"`
	Status     JobStatus      `json:"status"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
	Results    []TargetResult `json:"results"`
}
