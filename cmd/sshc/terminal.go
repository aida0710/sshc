package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"sshc/internal/api"
)

type terminalControlWire struct {
	Session    api.TerminalSession `json:"session"`
	Generation uint64              `json:"generation"`
	State      string              `json:"state"`
	Cursor     struct {
		Requested uint64 `json:"requested"`
		Start     uint64 `json:"start"`
		Next      uint64 `json:"next"`
		End       uint64 `json:"end"`
		Truncated bool   `json:"truncated"`
	} `json:"cursor"`
	Output string `json:"output"`
}

var (
	errTerminalSessionNotFound   = errors.New("terminal session was not found")
	errTerminalSelectorAmbiguous = errors.New("terminal session ID prefix is ambiguous")
	errTerminalWaitTimeout       = errors.New("terminal wait timed out")
	errTerminalDeliveryFailed    = errors.New("terminal command was not delivered")
)

func runTerminal(
	ctx context.Context, called terminalInvocation, stateDir string, client *http.Client,
	stdout, stderr io.Writer,
) int {
	engine, err := openEngineAPI(ctx, stateDir, client)
	if err != nil {
		return finishTerminalFailure(called.JSON, err, stdout, stderr)
	}
	defer func() { _ = engine.Close() }()

	result, err := executeTerminal(ctx, engine, called)
	if err != nil {
		return finishTerminalFailure(called.JSON, err, stdout, stderr)
	}
	if called.JSON {
		if err := writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: true, Result: result}); err != nil {
			return 1
		}
		return 0
	}
	writeTerminalResult(stdout, stderr, called, result)
	return 0
}

func executeTerminal(ctx context.Context, engine *engineAPI, called terminalInvocation) (any, error) {
	switch called.Action {
	case terminalList:
		return terminalSessions(ctx, engine)
	case terminalCreate:
		request := api.OpenTerminalSessionRequest{Kind: api.OpenTerminalSessionRequestKind(called.Kind)}
		if called.Alias != "" {
			request.Alias = &called.Alias
		}
		var opened api.OpenTerminalSessionResponse
		if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/terminal/sessions", request, &opened); err != nil {
			return nil, err
		}
		if opened.Session.Id == "" {
			return nil, errEngineInvalidResponse
		}
		// The stream ticket is a browser attachment credential. The control CLI
		// neither needs nor exposes it.
		opened.StreamTicket = ""
		return opened.Session, nil
	}

	sessions, err := terminalSessions(ctx, engine)
	if err != nil {
		return nil, err
	}
	session, err := resolveTerminalSession(sessions.Sessions, called.Selector)
	if err != nil {
		return nil, err
	}
	path := "/api/v1/terminal/sessions/" + url.PathEscape(session.Id)
	switch called.Action {
	case terminalShow:
		return readTerminalControl(ctx, engine, session.Id, 0, 0)
	case terminalRead:
		return readTerminalControl(ctx, engine, session.Id, called.Cursor, called.Limit)
	case terminalRename:
		var updated api.TerminalSessionList
		if err := engine.sendJSON(ctx, http.MethodPatch, path,
			api.RenameTerminalSessionRequest{Title: called.Title}, &updated); err != nil {
			return nil, err
		}
		return resolveExactTerminalSession(updated.Sessions, session.Id)
	case terminalClose:
		var updated api.TerminalSessionList
		if err := engine.sendJSON(ctx, http.MethodDelete, path, nil, &updated); err != nil {
			return nil, err
		}
		return map[string]any{"id": session.Id, "closed": true}, nil
	case terminalSend:
		return sendTerminalCommand(ctx, engine, session, called.Text, called.Submit)
	case terminalWait:
		return waitForTerminal(ctx, engine, session.Id, called.WaitFor, called.Timeout)
	default:
		return nil, errors.New("terminal action is not implemented")
	}
}

func terminalSessions(ctx context.Context, engine *engineAPI) (api.TerminalSessionList, error) {
	var listed api.TerminalSessionList
	if err := engine.getJSON(ctx, "/api/v1/terminal/sessions", &listed); err != nil {
		return api.TerminalSessionList{}, err
	}
	for _, session := range listed.Sessions {
		if session.Id == "" || len(session.Id) > 64 {
			return api.TerminalSessionList{}, errEngineInvalidResponse
		}
	}
	return listed, nil
}

func resolveTerminalSession(sessions []api.TerminalSession, selector string) (api.TerminalSession, error) {
	for _, session := range sessions {
		if session.Id == selector {
			return session, nil
		}
	}
	var found *api.TerminalSession
	for index := range sessions {
		if !strings.HasPrefix(sessions[index].Id, selector) {
			continue
		}
		if found != nil {
			return api.TerminalSession{}, errTerminalSelectorAmbiguous
		}
		copy := sessions[index]
		found = &copy
	}
	if found == nil {
		return api.TerminalSession{}, errTerminalSessionNotFound
	}
	return *found, nil
}

func resolveExactTerminalSession(sessions []api.TerminalSession, id string) (api.TerminalSession, error) {
	for _, session := range sessions {
		if session.Id == id {
			return session, nil
		}
	}
	return api.TerminalSession{}, errEngineInvalidResponse
}

func readTerminalControl(
	ctx context.Context, engine *engineAPI, id string, cursor uint64, limit int,
) (terminalControlWire, error) {
	path := "/api/v1/terminal/sessions/" + url.PathEscape(id) + "/control?cursor=" +
		strconv.FormatUint(cursor, 10) + "&limit=" + strconv.Itoa(limit)
	var control terminalControlWire
	if err := engine.getJSON(ctx, path, &control); err != nil {
		return terminalControlWire{}, err
	}
	if control.Session.Id != id || control.Generation == 0 || !validTerminalWaitState(control.State) ||
		!validTerminalControlCursor(control.Cursor.Requested, control.Cursor.Start, control.Cursor.Next,
			control.Cursor.End, control.Cursor.Truncated, cursor, limit) || !plainTerminalOutput(control.Output, limit) {
		return terminalControlWire{}, errEngineInvalidResponse
	}
	return control, nil
}

func validTerminalControlCursor(requested, start, next, end uint64, truncated bool, cursor uint64, limit int) bool {
	if requested != cursor || start > next || next > end || next-start > uint64(limit) {
		return false
	}
	if truncated {
		return start > cursor
	}
	return start == cursor
}

func plainTerminalOutput(output string, limit int) bool {
	if len(output) > limit {
		return false
	}
	for _, character := range output {
		if character == '\n' || character == '\t' {
			continue
		}
		if character == '\x1b' || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func sendTerminalCommand(
	ctx context.Context, engine *engineAPI, session api.TerminalSession, command string, submit bool,
) (api.TerminalCommandResult, error) {
	target := api.TerminalCommandTargetRequest{TargetId: "cli", SessionId: session.Id}
	request := api.TerminalCommandPreviewRequest{
		Command: &command, Inputs: map[string]string{}, Targets: []api.TerminalCommandTargetRequest{target},
		Submit: &submit,
	}
	var preview api.TerminalCommandPreview
	if err := engine.sendJSON(ctx, http.MethodPost, "/api/v1/terminal/commands/preview", request, &preview); err != nil {
		return api.TerminalCommandResult{}, err
	}
	if preview.Evidence == "" || preview.ActionToken == "" || len(preview.Targets) != 1 ||
		preview.Targets[0].SessionId != session.Id || preview.Targets[0].TargetId != target.TargetId {
		return api.TerminalCommandResult{}, errEngineInvalidResponse
	}
	dispatch := api.TerminalCommandDispatchRequest{
		Command: &command, Evidence: preview.Evidence, Inputs: map[string]string{},
		Targets: []api.TerminalCommandTargetRequest{target}, Submit: &submit,
	}
	var response api.TerminalCommandDispatchResponse
	if err := engine.sendJSONWithAction(ctx, http.MethodPost, "/api/v1/terminal/commands",
		preview.ActionToken, dispatch, &response); err != nil {
		return api.TerminalCommandResult{}, err
	}
	if len(response.Results) != 1 || response.Results[0].SessionId != session.Id ||
		response.Results[0].TargetId != target.TargetId {
		return api.TerminalCommandResult{}, errEngineInvalidResponse
	}
	if response.Results[0].Status != api.TerminalCommandResultStatusDelivered {
		return api.TerminalCommandResult{}, errTerminalDeliveryFailed
	}
	return response.Results[0], nil
}

func waitForTerminal(
	ctx context.Context, engine *engineAPI, id, wanted string, timeout time.Duration,
) (terminalControlWire, error) {
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		control, err := readTerminalControl(waitContext, engine, id, 0, 0)
		if err != nil {
			return terminalControlWire{}, err
		}
		if control.State == wanted {
			return control, nil
		}
		select {
		case <-waitContext.Done():
			if errors.Is(waitContext.Err(), context.DeadlineExceeded) {
				return terminalControlWire{}, errTerminalWaitTimeout
			}
			return terminalControlWire{}, waitContext.Err()
		case <-ticker.C:
		}
	}
}

func writeTerminalResult(stdout, stderr io.Writer, called terminalInvocation, result any) {
	switch value := result.(type) {
	case api.TerminalSessionList:
		sessions := append([]api.TerminalSession(nil), value.Sessions...)
		sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].StartedAt < sessions[j].StartedAt })
		for _, session := range sessions {
			alias := "-"
			if session.Alias != nil {
				alias = *session.Alias
			}
			fmt.Fprintf(stdout, "%s  %-17s %-5s  %s  %s\n", session.Id, session.State, session.Kind,
				safeTerminalCell(session.Title), safeTerminalCell(alias))
		}
	case terminalControlWire:
		if called.Action == terminalRead {
			if value.Cursor.Truncated {
				fmt.Fprintf(stderr, "sshc: scrollback before cursor %d is no longer retained; continuing at %d\n",
					value.Cursor.Requested, value.Cursor.Start)
			}
			fmt.Fprint(stdout, value.Output)
			return
		}
		fmt.Fprintf(stdout, "id          %s\nkind        %s\ntitle       %s\nstate       %s\ngeneration  %d\ncursor      %d\n",
			value.Session.Id, value.Session.Kind, safeTerminalCell(value.Session.Title), value.State,
			value.Generation, value.Cursor.End)
	case api.TerminalSession:
		fmt.Fprintf(stdout, "%s  %s  %s\n", value.Id, value.State, safeTerminalCell(value.Title))
	case api.TerminalCommandResult:
		fmt.Fprintf(stdout, "delivered  %s  %s\n", value.SessionId, safeTerminalCell(value.Title))
	case map[string]any:
		fmt.Fprintf(stdout, "closed  %s\n", value["id"])
	}
}

func finishTerminalFailure(asJSON bool, err error, stdout, stderr io.Writer) int {
	failure := classifyCommandFailure(err)
	switch {
	case errors.Is(err, errTerminalSessionNotFound):
		failure = commandFailure{Kind: "terminal_session_not_found", Retryable: false}
	case errors.Is(err, errTerminalSelectorAmbiguous):
		failure = commandFailure{Kind: "terminal_session_ambiguous", Retryable: false}
	case errors.Is(err, errTerminalWaitTimeout):
		failure = commandFailure{Kind: "terminal_wait_timeout", Retryable: true}
	case errors.Is(err, errTerminalDeliveryFailed):
		failure = commandFailure{Kind: "terminal_command_delivery_failed", Retryable: true}
	}
	exit := 1
	if errors.Is(err, context.Canceled) {
		exit = 130
	}
	if asJSON {
		_ = writeCommandEnvelope(stdout, commandEnvelope{SchemaVersion: 1, Success: false, Failure: &failure})
		return exit
	}
	switch failure.Kind {
	case "terminal_session_not_found":
		fmt.Fprintln(stderr, "sshc: no terminal matches that session ID")
	case "terminal_session_ambiguous":
		fmt.Fprintln(stderr, "sshc: that session ID prefix matches more than one terminal; use more characters")
	case "terminal_wait_timeout":
		fmt.Fprintln(stderr, "sshc: terminal wait timed out")
	case "terminal_command_delivery_failed", "terminal_command_target_unavailable", "terminal_command_target_changed":
		fmt.Fprintln(stderr, "sshc: the terminal changed or exited before the command could be delivered")
	case "vault_locked":
		fmt.Fprintln(stderr, "sshc: the vault is locked; run sshc vault unlock")
	case "engine_not_running":
		fmt.Fprintln(stderr, "sshc: no engine is running; start the desktop app or run sshc engine")
	default:
		fmt.Fprintf(stderr, "sshc: terminal operation failed (%s)\n", failure.Kind)
	}
	return exit
}
