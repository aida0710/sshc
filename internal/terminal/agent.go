package terminal

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// AgentKind is a coding-agent integration understood by the common bridge.
type AgentKind string

const (
	AgentClaude   AgentKind = "claude"
	AgentCodex    AgentKind = "codex"
	AgentOpenCode AgentKind = "opencode"
)

// AgentState is deliberately small. Product-specific lifecycle events are
// normalised by the managed integration before they reach the terminal.
type AgentState string

const (
	AgentWorking   AgentState = "working"
	AgentAttention AgentState = "attention"
	AgentReady     AgentState = "ready"
	AgentUnknown   AgentState = "unknown"
)

type AgentSignalKind string

const (
	AgentSignalAttention AgentSignalKind = "attention"
	AgentSignalCompleted AgentSignalKind = "completed"
)

type AgentSignal struct {
	Kind       AgentSignalKind
	OccurredAt time.Time
}

// AgentView contains display-only information. The opaque native session
// reference never crosses the terminal package boundary.
type AgentView struct {
	Kind               AgentKind
	State              AgentState
	CWD                string
	Model              string
	SessionName        string
	Resumable          bool
	ObservationVersion uint64
	SignalVersion      uint64
	LastSignal         *AgentSignal
}

type TitleSource string

const (
	TitleUser       TitleSource = "user"
	TitleAgent      TitleSource = "agent"
	TitleCandidate  TitleSource = "candidate"
	TitleConnection TitleSource = "connection"
	TitleFallback   TitleSource = "fallback"
)

type Presentation struct {
	DisplayTitle string
	TitleSource  TitleSource
	TitlePinned  bool
}

// agentEvent is the allowlisted version-1 payload carried by OSC 6973.
type agentEvent struct {
	Version int       `json:"v"`
	Agent   AgentKind `json:"agent"`
	Event   string    `json:"event"`
	Session string    `json:"session,omitempty"`
	CWD     string    `json:"cwd,omitempty"`
	Model   string    `json:"model,omitempty"`
	Name    string    `json:"name,omitempty"`
	Seq     uint64    `json:"seq"`
}

type agentObservation struct {
	AgentView
	reference     string
	seq           uint64
	observedAt    time.Time
	generation    uint64
	workingAt     time.Time
	lastAttention time.Time
}

type agentCandidate struct {
	kind       AgentKind
	reference  string
	name       string
	generation uint64
	observedAt time.Time
}

const (
	MaxAgentPayload        = 4096
	MaxAgentDisplayRunes   = 160
	AgentObservationTTL    = 5 * time.Minute
	AgentCompletionFloor   = 3 * time.Second
	AgentAttentionCooldown = 2 * time.Second
)

var agentReference = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)

var (
	errInvalidAgentEvent     = errors.New("invalid agent event")
	errInvalidAgentReference = errors.New("invalid agent session reference")
)

func validAgent(kind AgentKind) bool {
	return kind == AgentClaude || kind == AgentCodex || kind == AgentOpenCode
}

func validateAgentReference(_ AgentKind, reference string) error {
	if !agentReference.MatchString(reference) {
		return errInvalidAgentReference
	}
	return nil
}

// AgentResumeArgv is generated only from an adapter and a validated opaque
// reference. Neither browser input nor remote output can choose flags.
func AgentResumeArgv(kind AgentKind, reference string) (string, []string, error) {
	if !validAgent(kind) || validateAgentReference(kind, reference) != nil {
		return "", nil, errInvalidAgentReference
	}
	switch kind {
	case AgentClaude:
		return "claude", []string{"--resume", reference}, nil
	case AgentCodex:
		return "codex", []string{"resume", reference}, nil
	case AgentOpenCode:
		return "opencode", []string{"--session", reference}, nil
	default:
		return "", nil, errInvalidAgentReference
	}
}

// AgentResumeCommand is safe to pass to SSH's remote command field because the
// reference grammar excludes whitespace and shell metacharacters.
func AgentResumeCommand(kind AgentKind, reference string) (string, error) {
	executable, arguments, err := AgentResumeArgv(kind, reference)
	if err != nil {
		return "", err
	}
	return strings.Join(append([]string{executable}, arguments...), " "), nil
}

func decodeAgentEvent(payload []byte) (agentEvent, error) {
	var event agentEvent
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return agentEvent{}, errInvalidAgentEvent
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || event.Version != 1 || !validAgent(event.Agent) || event.Seq == 0 {
		return agentEvent{}, errInvalidAgentEvent
	}
	switch event.Event {
	case string(AgentWorking), string(AgentAttention), string(AgentReady), string(AgentUnknown), "ended":
	default:
		return agentEvent{}, errInvalidAgentEvent
	}
	if event.Session != "" {
		if err := validateAgentReference(event.Agent, event.Session); err != nil {
			return agentEvent{}, err
		}
	}
	event.CWD = cleanAgentDisplay(event.CWD, MaxAgentDisplayRunes)
	event.Model = cleanAgentDisplay(event.Model, MaxAgentDisplayRunes)
	event.Name = cleanAgentDisplay(event.Name, MaxTitle)
	return event, nil
}

func cleanAgentDisplay(value string, maximum int) string {
	value = strings.TrimSpace(norm.NFC.String(value))
	if value == "" {
		return ""
	}
	var cleaned strings.Builder
	count := 0
	for _, character := range value {
		if character == utf8.RuneError || unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			continue
		}
		cleaned.WriteRune(character)
		count++
		if count >= maximum {
			break
		}
	}
	return strings.TrimSpace(cleaned.String())
}
