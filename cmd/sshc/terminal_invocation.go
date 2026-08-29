package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type terminalAction uint8

const (
	terminalInvalid terminalAction = iota
	terminalList
	terminalShow
	terminalRead
	terminalSend
	terminalWait
	terminalCreate
	terminalRename
	terminalClose
)

type terminalInvocation struct {
	Action   terminalAction
	Selector string
	Kind     string
	Alias    string
	Title    string
	Text     string
	Cursor   uint64
	Limit    int
	WaitFor  string
	Timeout  time.Duration
	JSON     bool
	Submit   bool
}

const terminalCLIMaxReadBytes = 64 << 10

func parseTerminalInvocation(args []string) (invocation, error) {
	if len(args) == 0 {
		return invalidInvocation("terminal requires an action")
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return invocation{Kind: invocationHelp}, nil
	}
	parsed := terminalInvocation{Submit: true, Limit: 32 << 10, Timeout: 5 * time.Minute}
	action, rest := args[0], args[1:]
	switch action {
	case "list":
		parsed.Action = terminalList
		if err := parseTerminalJSONOnly(rest, &parsed); err != nil {
			return invalidInvocation(err.Error())
		}
	case "show", "close":
		if len(rest) < 1 {
			return invalidInvocation("terminal " + action + " requires a session ID")
		}
		parsed.Selector = rest[0]
		if err := validateTerminalSelector(parsed.Selector); err != nil {
			return invalidInvocation(err.Error())
		}
		if err := parseTerminalJSONOnly(rest[1:], &parsed); err != nil {
			return invalidInvocation(err.Error())
		}
		if action == "show" {
			parsed.Action = terminalShow
		} else {
			parsed.Action = terminalClose
		}
	case "read":
		parsed.Action = terminalRead
		if len(rest) < 1 {
			return invalidInvocation("terminal read requires a session ID")
		}
		parsed.Selector = rest[0]
		if err := validateTerminalSelector(parsed.Selector); err != nil {
			return invalidInvocation(err.Error())
		}
		for index := 1; index < len(rest); index++ {
			switch rest[index] {
			case "--cursor":
				index++
				if index >= len(rest) {
					return invalidInvocation("terminal read --cursor requires a number")
				}
				cursor, err := strconv.ParseUint(rest[index], 10, 64)
				if err != nil {
					return invalidInvocation("terminal read --cursor requires a number")
				}
				parsed.Cursor = cursor
			case "--limit":
				index++
				if index >= len(rest) {
					return invalidInvocation("terminal read --limit requires a number")
				}
				limit, err := strconv.Atoi(rest[index])
				if err != nil || limit < 0 || limit > terminalCLIMaxReadBytes {
					return invalidInvocation(fmt.Sprintf("terminal read --limit must be between 0 and %d", terminalCLIMaxReadBytes))
				}
				parsed.Limit = limit
			case "--json":
				if parsed.JSON {
					return invalidInvocation("terminal read accepts --json only once")
				}
				parsed.JSON = true
			default:
				return invalidInvocation("terminal read does not take " + rest[index])
			}
		}
	case "send":
		parsed.Action = terminalSend
		if len(rest) < 1 {
			return invalidInvocation("terminal send requires a session ID")
		}
		parsed.Selector = rest[0]
		if err := validateTerminalSelector(parsed.Selector); err != nil {
			return invalidInvocation(err.Error())
		}
		textSet := false
		for index := 1; index < len(rest); index++ {
			switch rest[index] {
			case "--text":
				index++
				if index >= len(rest) || textSet {
					return invalidInvocation("terminal send requires one --text value")
				}
				parsed.Text, textSet = rest[index], true
			case "--no-enter":
				if !parsed.Submit {
					return invalidInvocation("terminal send accepts --no-enter only once")
				}
				parsed.Submit = false
			case "--json":
				if parsed.JSON {
					return invalidInvocation("terminal send accepts --json only once")
				}
				parsed.JSON = true
			default:
				return invalidInvocation("terminal send does not take " + rest[index])
			}
		}
		if !textSet || parsed.Text == "" {
			return invalidInvocation("terminal send requires a non-empty --text value")
		}
	case "wait":
		parsed.Action = terminalWait
		if len(rest) < 1 {
			return invalidInvocation("terminal wait requires a session ID")
		}
		parsed.Selector = rest[0]
		if err := validateTerminalSelector(parsed.Selector); err != nil {
			return invalidInvocation(err.Error())
		}
		forSet := false
		for index := 1; index < len(rest); index++ {
			switch rest[index] {
			case "--for":
				index++
				if index >= len(rest) || forSet || !validTerminalWaitState(rest[index]) {
					return invalidInvocation("terminal wait --for requires an explicit lifecycle state")
				}
				parsed.WaitFor, forSet = rest[index], true
			case "--timeout":
				index++
				if index >= len(rest) {
					return invalidInvocation("terminal wait --timeout requires a duration")
				}
				timeout, err := time.ParseDuration(rest[index])
				if err != nil || timeout <= 0 || timeout > 24*time.Hour {
					return invalidInvocation("terminal wait --timeout must be greater than zero and at most 24h")
				}
				parsed.Timeout = timeout
			case "--json":
				if parsed.JSON {
					return invalidInvocation("terminal wait accepts --json only once")
				}
				parsed.JSON = true
			default:
				return invalidInvocation("terminal wait does not take " + rest[index])
			}
		}
		if !forSet {
			return invalidInvocation("terminal wait requires --for")
		}
	case "create":
		parsed.Action = terminalCreate
		if len(rest) < 1 || (rest[0] != "shell" && rest[0] != "ssh") {
			return invalidInvocation("terminal create requires shell or ssh")
		}
		parsed.Kind = rest[0]
		remaining := rest[1:]
		if parsed.Kind == "ssh" {
			if len(remaining) == 0 || remaining[0] == "--json" || strings.HasPrefix(remaining[0], "-") {
				return invalidInvocation("terminal create ssh requires an alias")
			}
			parsed.Alias, remaining = remaining[0], remaining[1:]
		}
		if err := parseTerminalJSONOnly(remaining, &parsed); err != nil {
			return invalidInvocation(err.Error())
		}
	case "rename":
		parsed.Action = terminalRename
		if len(rest) < 2 {
			return invalidInvocation("terminal rename requires a session ID and title")
		}
		parsed.Selector, parsed.Title = rest[0], rest[1]
		if err := validateTerminalSelector(parsed.Selector); err != nil {
			return invalidInvocation(err.Error())
		}
		if strings.TrimSpace(parsed.Title) == "" {
			return invalidInvocation("terminal rename requires a non-empty title")
		}
		if err := parseTerminalJSONOnly(rest[2:], &parsed); err != nil {
			return invalidInvocation(err.Error())
		}
	default:
		return invalidInvocation(fmt.Sprintf("unknown terminal action %q", action))
	}
	return invocation{Kind: invocationTerminal, JSON: parsed.JSON, Terminal: &parsed}, nil
}

func parseTerminalJSONOnly(args []string, parsed *terminalInvocation) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && args[0] == "--json" {
		parsed.JSON = true
		return nil
	}
	return fmt.Errorf("terminal action only accepts optional --json")
}

func validateTerminalSelector(selector string) error {
	if len(selector) < 8 || len(selector) > 64 {
		return fmt.Errorf("terminal session ID must be an ID or unique prefix of at least 8 hexadecimal characters")
	}
	for _, character := range selector {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("terminal session ID must use lowercase hexadecimal characters")
		}
	}
	return nil
}

func validTerminalWaitState(state string) bool {
	switch state {
	case "connecting", "connected", "reconnecting", "exited",
		"agent-working", "agent-attention", "agent-ready", "agent-ended":
		return true
	default:
		return false
	}
}
