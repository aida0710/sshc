package httpserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	"sshc/internal/snippets"
	"sshc/internal/terminal"
)

const terminalCommandActionTarget = "live-terminal-sessions"

type terminalCommandTargetRequest = api.TerminalCommandTargetRequest
type terminalCommandPreviewRequest = api.TerminalCommandPreviewRequest
type terminalCommandDispatchRequest = api.TerminalCommandDispatchRequest
type terminalCommandPreviewTarget = api.TerminalCommandPreviewTarget
type terminalCommandPreviewResponse = api.TerminalCommandPreview
type terminalCommandResult = api.TerminalCommandResult
type terminalCommandStatus = api.TerminalCommandResultStatus
type terminalCommandDispatchResponse = api.TerminalCommandDispatchResponse

func terminalCommandPreviewFromDispatch(r terminalCommandDispatchRequest) terminalCommandPreviewRequest {
	return terminalCommandPreviewRequest{
		SnippetId: r.SnippetId, Command: r.Command, Inputs: r.Inputs, Targets: r.Targets,
	}
}

const (
	terminalCommandDelivered = api.TerminalCommandResultStatusDelivered
	terminalCommandFailed    = api.TerminalCommandResultStatusFailed
)

type plannedTerminalCommand struct {
	request terminalCommandTargetRequest
	target  terminal.CommandTarget
}

type terminalCommandPlan struct {
	command  snippets.PreparedCommand
	targets  []plannedTerminalCommand
	evidence string
}

func (h TerminalHandlers) commandPlan(request terminalCommandPreviewRequest) (terminalCommandPlan, error) {
	if h.Snippets == nil || len(request.Targets) == 0 || len(request.Targets) > snippets.MaxTargets {
		return terminalCommandPlan{}, snippets.ErrInvalidTarget
	}
	command, err := h.Snippets.PrepareTerminalCommand(snippets.CommandRequest{
		SnippetID: stringValue(request.SnippetId), Command: stringValue(request.Command), Inputs: request.Inputs,
	})
	if err != nil {
		return terminalCommandPlan{}, err
	}
	if len(command.Command) > terminal.MaxCommandBytes {
		return terminalCommandPlan{}, terminal.ErrCommandTooLarge
	}

	seenTargets := make(map[string]bool, len(request.Targets))
	seenSessions := make(map[string]bool, len(request.Targets))
	plan := terminalCommandPlan{command: command, targets: make([]plannedTerminalCommand, 0, len(request.Targets))}
	for _, requested := range request.Targets {
		if requested.TargetId == "" || len(requested.TargetId) > 255 || strings.TrimSpace(requested.TargetId) != requested.TargetId || strings.IndexByte(requested.TargetId, 0) >= 0 ||
			requested.SessionId == "" || len(requested.SessionId) > maxSessionIdentifier || strings.TrimSpace(requested.SessionId) != requested.SessionId || strings.IndexByte(requested.SessionId, 0) >= 0 ||
			seenTargets[requested.TargetId] || seenSessions[requested.SessionId] {
			return terminalCommandPlan{}, snippets.ErrInvalidTarget
		}
		seenTargets[requested.TargetId] = true
		seenSessions[requested.SessionId] = true
		target, err := h.Registry.CommandTarget(requested.SessionId)
		if err != nil {
			return terminalCommandPlan{}, err
		}
		plan.targets = append(plan.targets, plannedTerminalCommand{request: requested, target: target})
	}

	payload := struct {
		CommandEvidence string `json:"commandEvidence"`
		Targets         []struct {
			TargetID   string `json:"targetId"`
			SessionID  string `json:"sessionId"`
			Alias      string `json:"alias"`
			Title      string `json:"title"`
			Generation uint64 `json:"generation"`
		} `json:"targets"`
	}{CommandEvidence: command.Evidence}
	for _, target := range plan.targets {
		payload.Targets = append(payload.Targets, struct {
			TargetID   string `json:"targetId"`
			SessionID  string `json:"sessionId"`
			Alias      string `json:"alias"`
			Title      string `json:"title"`
			Generation uint64 `json:"generation"`
		}{
			TargetID: target.request.TargetId, SessionID: target.target.ID, Alias: target.target.Alias,
			Title: target.target.Title, Generation: target.target.Generation,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return terminalCommandPlan{}, err
	}
	digest := sha256.Sum256(encoded)
	plan.evidence = hex.EncodeToString(digest[:])
	return plan, nil
}

func terminalCommandProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, snippets.ErrSecretTerminal):
		return problem(c, http.StatusBadRequest, "terminal_command_secret_refused")
	case errors.Is(err, terminal.ErrCommandTooLarge):
		return problem(c, http.StatusBadRequest, "terminal_command_too_large")
	case errors.Is(err, snippets.ErrInvalidSnippet), errors.Is(err, snippets.ErrInvalidVariable),
		errors.Is(err, snippets.ErrUnknownVariable), errors.Is(err, snippets.ErrMissingVariable),
		errors.Is(err, snippets.ErrMalformedTemplate), errors.Is(err, snippets.ErrUnknownSnippet),
		errors.Is(err, snippets.ErrInvalidTarget), errors.Is(err, snippets.ErrDuplicateTarget):
		return problem(c, http.StatusBadRequest, "invalid_terminal_command")
	case errors.Is(err, terminal.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	case errors.Is(err, terminal.ErrWrongKind):
		return problem(c, http.StatusConflict, "terminal_command_ssh_only")
	case errors.Is(err, terminal.ErrNotConnected), errors.Is(err, terminal.ErrGenerationChanged),
		errors.Is(err, terminal.ErrExactInputUnavailable):
		return problem(c, http.StatusConflict, "terminal_command_target_unavailable")
	default:
		return problem(c, http.StatusInternalServerError, "terminal_command_failed")
	}
}

func (h TerminalHandlers) PreviewCommand(c *echo.Context) error {
	var request terminalCommandPreviewRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	plan, err := h.commandPlan(request)
	if err != nil {
		return terminalCommandProblem(c, err)
	}
	issued, err := h.Actions.issueEvidence(c, session.ActionTerminalCommand, terminalCommandActionTarget, plan.evidence)
	if err != nil {
		return err
	}
	targets := make([]terminalCommandPreviewTarget, 0, len(plan.targets))
	for _, target := range plan.targets {
		targets = append(targets, terminalCommandPreviewTarget{
			TargetId: target.request.TargetId, SessionId: target.target.ID, Alias: target.target.Alias,
			Title: target.target.Title, Command: plan.command.Display,
		})
	}
	return c.JSON(http.StatusOK, terminalCommandPreviewResponse{
		SnippetId: plan.command.SnippetID, Evidence: plan.evidence, ActionToken: issued.Token,
		ActionExpiresAt: issued.ExpiresAt, Targets: targets,
	})
}

func (h TerminalHandlers) DispatchCommand(c *echo.Context) error {
	var request terminalCommandDispatchRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(request.Evidence) != sha256.Size*2 {
		return problem(c, http.StatusBadRequest, "invalid_terminal_command")
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionTerminalCommand, terminalCommandActionTarget, request.Evidence); !allowed {
		return response
	}
	plan, err := h.commandPlan(terminalCommandPreviewFromDispatch(request))
	if err != nil || subtle.ConstantTimeCompare([]byte(plan.evidence), []byte(request.Evidence)) != 1 {
		return problem(c, http.StatusConflict, "terminal_command_preview_changed")
	}

	results := make([]terminalCommandResult, 0, len(plan.targets))
	for _, planned := range plan.targets {
		result := terminalCommandResult{
			TargetId: planned.request.TargetId, SessionId: planned.target.ID,
			Alias: planned.target.Alias, Title: planned.target.Title, Status: terminalCommandDelivered,
		}
		writeContext, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
		err := h.Registry.WriteCommand(writeContext, planned.target, plan.command.Command)
		cancel()
		if err != nil {
			result.Status = terminalCommandFailed
			switch {
			case errors.Is(err, terminal.ErrNotFound):
				result.Problem = stringPointer("terminal_session_not_found")
			case errors.Is(err, terminal.ErrWrongKind):
				result.Problem = stringPointer("terminal_command_ssh_only")
			case errors.Is(err, terminal.ErrNotConnected), errors.Is(err, terminal.ErrGenerationChanged):
				result.Problem = stringPointer("terminal_command_target_changed")
			default:
				result.Problem = stringPointer("terminal_command_delivery_failed")
			}
		}
		results = append(results, result)
	}
	return c.JSON(http.StatusOK, terminalCommandDispatchResponse{Results: results})
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	return &value
}
