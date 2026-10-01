package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"sshc/internal/httpserver"
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
	// UnpinTitle は rename --auto で立つ。Title の代わりに表示名の固定を外し、
	// 画面の「Use automatic name」と同じく自動の名前へ戻す。
	UnpinTitle bool
	Text       string
	Cursor     uint64
	Limit      int
	WaitFor    string
	Timeout    time.Duration
	JSON       bool
	Submit     bool
}

const (
	// terminal read --limit の既定と上限は engine の /control と同じにする。範囲の
	// 正本は engine で、CLI が狭く断ったり、engine が断る値を送ったりしない。
	terminalCLIDefaultReadBytes = httpserver.DefaultTerminalControlReadBytes
	terminalCLIMaxReadBytes     = httpserver.MaxTerminalControlReadBytes
	// terminalCLIDefaultWaitTimeout は terminal wait --timeout の既定である。接続と
	// ログインを待つには足り、状態が変わらないまま止まったスクリプトを長く残さない。
	terminalCLIDefaultWaitTimeout = 5 * time.Minute
	// terminalCLIMaxWaitTimeout は terminal wait --timeout の上限である。単位の
	// 打ち間違い（24h のつもりの 240h など）で何日も待ち続けないためのもの。
	terminalCLIMaxWaitTimeout = 24 * time.Hour
)

var terminalActionNames = map[string]terminalAction{
	"list": terminalList, "show": terminalShow, "read": terminalRead, "send": terminalSend,
	"wait": terminalWait, "create": terminalCreate, "rename": terminalRename, "close": terminalClose,
}

var (
	terminalReadOptions = commandOptions{command: "terminal read", options: []commandOption{
		valueOption("--cursor"), valueOption("--limit"), jsonOption,
	}}
	terminalSendOptions = commandOptions{command: "terminal send", options: []commandOption{
		valueOption("--text"), switchOption("--no-enter"), jsonOption,
	}}
	terminalWaitOptions = commandOptions{command: "terminal wait", options: []commandOption{
		valueOption("--for"), valueOption("--timeout"), jsonOption,
	}}
	// terminalRenameOptions は "--" の後ろをオプションとして読まない。"-dev" のように "-" で
	// 始まる名前は、ほかの位置ではオプションとして断られるので、"--" の後ろに書いて付ける。
	terminalRenameOptions = commandOptions{command: "terminal rename", options: []commandOption{
		switchOption("--auto"), jsonOption,
	}, endsAtDelimiter: true}
	terminalReadLimitBounds = integerBounds{minimum: 0, maximum: terminalCLIMaxReadBytes, kind: "a number"}
)

func parseTerminalInvocation(args []string) (invocation, error) {
	if len(args) == 0 {
		return invalidInvocation("terminal requires an action")
	}
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandTerminal)), nil
	}
	if len(args) > 1 && validTerminalAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandTerminal) + " " + args[0]), nil
	}
	action, known := terminalActionNames[args[0]]
	if !known {
		return invalidInvocation(fmt.Sprintf("unknown terminal action %q", args[0]))
	}
	command, rest := "terminal "+args[0], args[1:]
	parsed := terminalInvocation{
		Action: action, Submit: true, Limit: terminalCLIDefaultReadBytes, Timeout: terminalCLIDefaultWaitTimeout,
	}
	if action != terminalList && action != terminalCreate {
		selector, remaining, err := takeTerminalSelector(command, rest)
		if err != nil {
			return invalidInvocation(err.Error())
		}
		parsed.Selector, rest = selector, remaining
	}
	var err error
	switch action {
	case terminalList, terminalShow, terminalClose:
		parsed.JSON, err = parseOptionalJSON(command, rest)
	case terminalRead:
		err = readTerminalReadOptions(rest, &parsed)
	case terminalSend:
		err = readTerminalSendOptions(rest, &parsed)
	case terminalWait:
		err = readTerminalWaitOptions(rest, &parsed)
	case terminalCreate:
		err = readTerminalCreate(rest, &parsed)
	case terminalRename:
		err = readTerminalRename(rest, &parsed)
	}
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationTerminal, JSON: parsed.JSON, Terminal: &parsed}, nil
}

// takeTerminalSelector は、操作名の直後に置くセッション ID を取り出し、残りの引数を返す。
func takeTerminalSelector(command string, args []string) (string, []string, error) {
	if len(args) < 1 {
		return "", nil, fmt.Errorf("%s requires a session ID", command)
	}
	if err := validateTerminalSelector(args[0]); err != nil {
		return "", nil, err
	}
	return args[0], args[1:], nil
}

func readTerminalReadOptions(args []string, parsed *terminalInvocation) error {
	arguments, err := terminalReadOptions.parseWithoutPositionals(args)
	if err != nil {
		return err
	}
	if value, found := arguments.value("--cursor"); found {
		cursor, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("terminal read --cursor requires a number")
		}
		parsed.Cursor = cursor
	}
	if parsed.Limit, err = arguments.integer("--limit", terminalReadLimitBounds, parsed.Limit); err != nil {
		return err
	}
	parsed.JSON = arguments.has("--json")
	return nil
}

func readTerminalSendOptions(args []string, parsed *terminalInvocation) error {
	arguments, err := terminalSendOptions.parseWithoutPositionals(args)
	if err != nil {
		return err
	}
	text, found := arguments.value("--text")
	if !found || text == "" {
		return fmt.Errorf("terminal send requires a non-empty --text value")
	}
	parsed.Text = text
	parsed.Submit = !arguments.has("--no-enter")
	parsed.JSON = arguments.has("--json")
	return nil
}

func readTerminalWaitOptions(args []string, parsed *terminalInvocation) error {
	arguments, err := terminalWaitOptions.parseWithoutPositionals(args)
	if err != nil {
		return err
	}
	state, found := arguments.value("--for")
	if !found {
		return fmt.Errorf("terminal wait requires --for")
	}
	if !validTerminalWaitState(state) {
		return fmt.Errorf("terminal wait --for requires an explicit lifecycle state")
	}
	parsed.WaitFor = state
	if value, found := arguments.value("--timeout"); found {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 || timeout > terminalCLIMaxWaitTimeout {
			return fmt.Errorf("terminal wait --timeout must be greater than zero and at most %d hours",
				int(terminalCLIMaxWaitTimeout/time.Hour))
		}
		parsed.Timeout = timeout
	}
	parsed.JSON = arguments.has("--json")
	return nil
}

// readTerminalRename は、付ける名前か --auto のどちらか一方を読む。名前は "--" の前の
// 位置引数と "--" の後ろの引数を区別せず、合わせて 1 つだけ受ける。
func readTerminalRename(args []string, parsed *terminalInvocation) error {
	arguments, err := terminalRenameOptions.parse(args)
	if err != nil {
		return err
	}
	parsed.JSON = arguments.has("--json")
	parsed.UnpinTitle = arguments.has("--auto")
	titles := slices.Concat(arguments.positionals, arguments.rest)
	switch {
	case len(titles) > 1:
		return fmt.Errorf("terminal rename does not take %q", titles[1])
	case parsed.UnpinTitle && len(titles) == 1:
		return fmt.Errorf("terminal rename cannot combine a title and --auto")
	case parsed.UnpinTitle:
		return nil
	case len(titles) == 0 || strings.TrimSpace(titles[0]) == "":
		return fmt.Errorf("terminal rename requires a non-empty title or --auto")
	}
	parsed.Title = titles[0]
	return nil
}

func readTerminalCreate(args []string, parsed *terminalInvocation) error {
	if len(args) < 1 || (args[0] != "shell" && args[0] != "ssh") {
		return fmt.Errorf("terminal create requires shell or ssh")
	}
	parsed.Kind = args[0]
	remaining := args[1:]
	if parsed.Kind == "ssh" {
		if len(remaining) == 0 || strings.HasPrefix(remaining[0], "-") {
			return fmt.Errorf("terminal create ssh requires an alias")
		}
		parsed.Alias, remaining = remaining[0], remaining[1:]
	}
	var err error
	parsed.JSON, err = parseOptionalJSON("terminal create", remaining)
	return err
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
