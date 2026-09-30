package main

import (
	"fmt"
	"io"
	"strings"

	"sshc/internal/validate"
)

type invocationKind uint8

const (
	invocationInvalid invocationKind = iota
	invocationOpenInBrowser
	invocationEngine
	invocationConnect
	invocationRun
	invocationChoose
	invocationList
	invocationOpen
	invocationStatus
	invocationVault
	invocationUpdate
	invocationService
	invocationOTP
	invocationVPN
	invocationHelp
	invocationVersion
	invocationTransport
	invocationRunTransport
	invocationInfo
	invocationSync
	invocationTerminal
	invocationSFTP
	invocationCompletion
)

type invocation struct {
	Kind invocationKind
	Args []string
	// HelpTopic は個別helpで表示するcommand path。空文字列は全体helpを表す。
	HelpTopic string
	// Port は `sshc engine --port N` の待受ポートで、0 は未指定を表す。
	Port int
	// Replace は既存の engine を確認なしで停止して置き換える。
	Replace bool
	// JSON は `sshc status --json` の機械可読出力を選択する。
	JSON bool
	// Yes は変更内容の確認を省略する。
	Yes       bool
	Transport *transportInvocation
	Sync      *syncInvocation
	Terminal  *terminalInvocation
	SFTP      *sftpInvocation
	OTP       *otpInvocation
	VPN       *vpnInvocation
}

// parseInvocation は、コマンドが誰の責務を求めるかを副作用なしに決める。
//
// alias より先に予約語を一度だけ読むのは、呼び出し側ごとに判定を持つと `engine`
// のような所有者指定が誤って SSH 接続先になり得るためである。ここで確定した Kind
// だけを dispatch すれば、引数の形と実行時の責務がずれない。
func parseInvocation(argv []string) (invocation, error) {
	if len(argv) == 0 {
		return invalidInvocation("missing program name")
	}
	if len(argv) == 1 {
		return invocation{Kind: invocationOpenInBrowser}, nil
	}

	word := argv[1]
	args := argv[2:]
	switch generatedCLICommand(word) {
	case cliCommandEngine:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandEngine)), nil
		}
		return parseEngineFlags(args)
	case cliCommandSerial:
		return parseTransportInvocation(transportSerial, args)
	case cliCommandTelnet:
		return parseTransportInvocation(transportTelnet, args)
	case cliCommandSsh:
		return parseSSHInvocation(args)
	case cliCommandInfo:
		return parseInfoInvocation(args)
	case cliCommandSync:
		return parseSyncInvocation(args)
	case cliCommandTerminal:
		return parseTerminalInvocation(args)
	case cliCommandSftp:
		return parseSFTPInvocation(args)
	case cliCommandCompletion:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandCompletion)), nil
		}
		if len(args) != 1 || !validCompletionShell(args[0]) {
			return invalidInvocation("completion requires bash, zsh, or fish")
		}
		return invocation{Kind: invocationCompletion, Args: copyInvocationArgs(args)}, nil
	case cliCommandOpen:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandOpen)), nil
		}
		return noArguments(invocationOpen, word, args)
	case cliCommandStatus:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandStatus)), nil
		}
		asJSON, err := parseOptionalJSON(word, args)
		if err != nil {
			return invalidInvocation(err.Error())
		}
		return invocation{Kind: invocationStatus, JSON: asJSON}, nil
	case cliCommandUpdate:
		if helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandUpdate)), nil
		}
		return parseConfirmationOnlyInvocation(invocationUpdate, word, args)
	case cliCommandService:
		return parseServiceInvocation(args)
	case cliCommandOtp:
		return parseOTPInvocation(args)
	case cliCommandVpn:
		return parseVPNInvocation(args)
	case cliCommandVault:
		return parseVaultInvocation(args)
	case cliCommandHelp:
		return parseHelpInvocation(word, args)
	// 旗も語も、同じところへ着く。`sshc version` が正式だが、`--version` は
	// 誰もが最初に打つ形である。受けないと、入れた直後の一行目が usage と
	// 終了コード 2 になる。
	case cliCommandVersion:
		if word == canonicalCLICommand(cliCommandVersion) && helpRequested(args) {
			return helpInvocation(canonicalCLICommand(cliCommandVersion)), nil
		}
		return noArguments(invocationVersion, word, args)
	}

	return invalidInvocation(fmt.Sprintf("unknown command %q", word))
}

// parseServiceInvocation は、`sshc service <install|status|disable>` を読む。
func parseServiceInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandService)), nil
	}
	if len(args) > 1 && validServiceAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandService) + " " + args[0]), nil
	}
	if len(args) == 0 || !validServiceAction(args[0]) {
		return invalidInvocation("service requires install, status, or disable")
	}
	action := args[0]
	if action == "status" {
		if len(args) != 1 {
			return invalidInvocation("service status takes no flags")
		}
		return invocation{Kind: invocationService, Args: []string{action}}, nil
	}
	parsed, err := parseConfirmationOnlyInvocation(invocationService, "service "+action, args[1:])
	if err != nil {
		return parsed, err
	}
	parsed.Args = []string{action}
	return parsed, nil
}

// parseVaultInvocation は、`sshc vault <action>` を読む。
func parseVaultInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandVault)), nil
	}
	if len(args) > 1 && validVaultAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(canonicalCLICommand(cliCommandVault) + " " + args[0]), nil
	}
	if len(args) != 1 {
		return invalidInvocation("vault requires one action")
	}
	switch args[0] {
	case "status", "create", "unlock", "lock", "change-password":
		return invocation{Kind: invocationVault, Args: copyInvocationArgs(args)}, nil
	default:
		return invalidInvocation(fmt.Sprintf("unknown vault action %q", args[0]))
	}
}

// parseHelpInvocation は、`sshc help [topic]` と `sshc -h`・`sshc --help` を読む。
func parseHelpInvocation(word string, args []string) (invocation, error) {
	if word == "-h" || word == "--help" {
		return noArguments(invocationHelp, word, args)
	}
	if len(args) == 0 {
		return helpInvocation(""), nil
	}
	topic := strings.Join(args, " ")
	if !validHelpTopic(topic) {
		return invalidInvocation(fmt.Sprintf("unknown help topic %q", topic))
	}
	return helpInvocation(topic), nil
}

func parseConfirmationOnlyInvocation(kind invocationKind, command string, args []string) (invocation, error) {
	yes, err := parseOptionalYes(command, args)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: kind, Yes: yes}, nil
}

func helpInvocation(topic string) invocation {
	return invocation{Kind: invocationHelp, HelpTopic: topic}
}

func helpRequested(args []string) bool {
	return len(args) > 0 && isHelpFlag(args[0])
}

func isHelpFlag(argument string) bool {
	return argument == "-h" || argument == "--help"
}

func parseInfoInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandInfo)), nil
	}
	if len(args) < 1 || args[0] == "" || args[0][0] == '-' {
		return invalidInvocation("info requires one alias followed by optional --json")
	}
	asJSON, err := parseOptionalJSON("info", args[1:])
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationInfo, Args: []string{args[0]}, JSON: asJSON}, nil
}

// parseSSHInvocation は alias を ssh namespace の内側だけで解釈する。
// transport や将来の top-level command と同名でも、alias の意味は変わらない。
func parseSSHInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(canonicalCLICommand(cliCommandSsh)), nil
	}
	if len(args) == 0 {
		return invocation{Kind: invocationChoose}, nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "--list":
			return invocation{Kind: invocationList}, nil
		}
		if args[0] == "" || args[0][0] == '-' {
			return invalidInvocation("ssh requires an alias")
		}
		return invocation{Kind: invocationConnect, Args: copyInvocationArgs(args)}, nil
	}
	if args[0] == "" || args[0][0] == '-' {
		return invalidInvocation("non-interactive SSH requires an alias before --non-interactive")
	}
	if len(args) < 4 || args[1] != "--non-interactive" || args[2] != "--" {
		return invalidInvocation("non-interactive SSH requires an alias, --non-interactive, --, and a command")
	}
	return invocation{Kind: invocationRun, Args: append([]string{args[0]}, args[3:]...)}, nil
}

var engineCommandOptions = commandOptions{command: "engine", options: []commandOption{
	switchOption("--replace"), valueOption("--port"),
}}

// enginePortBounds は、`sshc engine --port` が受ける範囲である。範囲は engine の設定の
// 検査（validate.EnginePort）と同じ出どころから取る。
var enginePortBounds = integerBounds{minimum: validate.MinEnginePort, maximum: validate.MaxEnginePort, kind: "a number"}

// parseEngineFlags は `sshc engine` のオプションを解析する。
func parseEngineFlags(args []string) (invocation, error) {
	arguments, err := engineCommandOptions.parseWithoutPositionals(args)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	// 無効な範囲と bind エラーを区別するため、ここで範囲を検証する。
	port, err := arguments.integer("--port", enginePortBounds, 0)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	return invocation{Kind: invocationEngine, Port: port, Replace: arguments.has("--replace")}, nil
}

func noArguments(kind invocationKind, command string, args []string) (invocation, error) {
	if len(args) != 0 {
		return invalidInvocation(fmt.Sprintf("%s takes no arguments", command))
	}
	return invocation{Kind: kind}, nil
}

func copyInvocationArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	return append([]string(nil), args...)
}

func invalidInvocation(reason string) (invocation, error) {
	return invocation{Kind: invocationInvalid}, fmt.Errorf("usage: %s", reason)
}

// usage は生成済みのCLI契約を出力する。
func usage(out io.Writer) {
	fmt.Fprint(out, generatedGlobalHelp)
}

func usageFor(out io.Writer, topic string) {
	if topic == "" {
		usage(out)
		return
	}
	fmt.Fprint(out, generatedCLIHelp[topic])
}
