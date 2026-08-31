package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

type invocationKind uint8

const (
	invocationInvalid invocationKind = iota
	invocationDesktop
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
	invocationHelp
	invocationVersion
	invocationTransport
	invocationRunTransport
	invocationInfo
	invocationSync
	invocationTerminal
)

type syncAction uint8

const (
	syncInvalid syncAction = iota
	syncStatus
	syncSetup
	syncPush
	syncPull
	syncNow
	syncAuto
)

type syncInvocation struct {
	Action  syncAction
	Force   bool
	JSON    bool
	Enabled bool
}

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
	JSON      bool
	Transport *transportInvocation
	Sync      *syncInvocation
	Terminal  *terminalInvocation
}

const (
	engineSubcommand   = "engine"
	vaultSubcommand    = "vault"
	helpSubcommand     = "help"
	versionSubcommand  = "version"
	updateSubcommand   = "update"
	serviceSubcommand  = "service"
	infoSubcommand     = "info"
	syncSubcommand     = "sync"
	terminalSubcommand = "terminal"
	StatusSubcommand   = "status"
	serialSubcommand   = "serial"
	telnetSubcommand   = "telnet"
	sshSubcommand      = "ssh"
)

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
		return invocation{Kind: invocationDesktop}, nil
	}

	word := argv[1]
	args := argv[2:]
	switch word {
	case engineSubcommand:
		if helpRequested(args) {
			return helpInvocation(engineSubcommand), nil
		}
		return parseEngineFlags(args)
	case serialSubcommand:
		return parseTransportInvocation(transportSerial, args)
	case telnetSubcommand:
		return parseTransportInvocation(transportTelnet, args)
	case sshSubcommand:
		return parseSSHInvocation(args)
	case infoSubcommand:
		return parseInfoInvocation(args)
	case syncSubcommand:
		return parseSyncInvocation(args)
	case terminalSubcommand:
		return parseTerminalInvocation(args)
	case OpenSubcommand:
		if helpRequested(args) {
			return helpInvocation(OpenSubcommand), nil
		}
		return noArguments(invocationOpen, word, args)
	case StatusSubcommand:
		if helpRequested(args) {
			return helpInvocation(StatusSubcommand), nil
		}
		if len(args) == 1 && args[0] == "--json" {
			return invocation{Kind: invocationStatus, JSON: true}, nil
		}
		return noArguments(invocationStatus, word, args)
	case updateSubcommand:
		if helpRequested(args) {
			return helpInvocation(updateSubcommand), nil
		}
		return noArguments(invocationUpdate, word, args)
	case serviceSubcommand:
		if helpRequested(args) {
			return helpInvocation(serviceSubcommand), nil
		}
		if len(args) > 1 && validServiceAction(args[0]) && isHelpFlag(args[1]) {
			return helpInvocation(serviceSubcommand + " " + args[0]), nil
		}
		if len(args) != 1 || !validServiceAction(args[0]) {
			return invalidInvocation("service requires install, status, or disable")
		}
		return invocation{Kind: invocationService, Args: copyInvocationArgs(args)}, nil
	case vaultSubcommand:
		if helpRequested(args) {
			return helpInvocation(vaultSubcommand), nil
		}
		if len(args) > 1 && validVaultAction(args[0]) && isHelpFlag(args[1]) {
			return helpInvocation(vaultSubcommand + " " + args[0]), nil
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
	case helpSubcommand:
		if len(args) == 0 {
			return helpInvocation(""), nil
		}
		topic := strings.Join(args, " ")
		if !validHelpTopic(topic) {
			return invalidInvocation(fmt.Sprintf("unknown help topic %q", topic))
		}
		return helpInvocation(topic), nil
	case "-h", "--help":
		return noArguments(invocationHelp, word, args)
	// 旗も語も、同じところへ着く。`sshc version` が正式だが、`--version` は
	// 誰もが最初に打つ形である。受けないと、入れた直後の一行目が usage と
	// 終了コード 2 になる。実際 docs/release-install.md はそれを案内していた。
	case versionSubcommand, "-v", "--version":
		if word == versionSubcommand && helpRequested(args) {
			return helpInvocation(versionSubcommand), nil
		}
		return noArguments(invocationVersion, word, args)
	}

	return invalidInvocation(fmt.Sprintf("unknown command %q", word))
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

func validVaultAction(action string) bool {
	switch action {
	case "status", "create", "unlock", "lock", "change-password":
		return true
	default:
		return false
	}
}

func validServiceAction(action string) bool {
	switch action {
	case "install", "status", "disable":
		return true
	default:
		return false
	}
}

func parseInfoInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(infoSubcommand), nil
	}
	if len(args) < 1 || len(args) > 2 || args[0] == "" || args[0][0] == '-' {
		return invalidInvocation("info requires one alias followed by optional --json")
	}
	called := invocation{Kind: invocationInfo, Args: []string{args[0]}}
	if len(args) == 2 {
		if args[1] != "--json" {
			return invalidInvocation("info only accepts --json after the alias")
		}
		called.JSON = true
	}
	return called, nil
}

func parseSyncInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(syncSubcommand), nil
	}
	if len(args) > 1 && validSyncAction(args[0]) && isHelpFlag(args[1]) {
		return helpInvocation(syncSubcommand + " " + args[0]), nil
	}
	if len(args) == 0 {
		return newSyncInvocation(syncInvocation{Action: syncStatus}), nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return newSyncInvocation(syncInvocation{Action: syncStatus, JSON: true}), nil
	}

	action := args[0]
	rest := args[1:]
	switch action {
	case "setup":
		if len(rest) != 0 {
			return invalidInvocation("sync setup takes no flags")
		}
		return newSyncInvocation(syncInvocation{Action: syncSetup}), nil
	case "push", "pull":
		parsed := syncInvocation{Action: syncPush}
		if action == "pull" {
			parsed.Action = syncPull
		}
		for _, flag := range rest {
			switch flag {
			case "--force":
				if parsed.Force {
					return invalidInvocation("sync accepts --force only once")
				}
				parsed.Force = true
			case "--json":
				if parsed.JSON {
					return invalidInvocation("sync accepts --json only once")
				}
				parsed.JSON = true
			default:
				return invalidInvocation("sync " + action + " does not take " + flag)
			}
		}
		return newSyncInvocation(parsed), nil
	case "now":
		asJSON, err := parseOptionalJSON(rest, "sync now")
		if err != nil {
			return invocation{Kind: invocationInvalid}, err
		}
		return newSyncInvocation(syncInvocation{Action: syncNow, JSON: asJSON}), nil
	case "auto":
		if len(rest) < 1 || (rest[0] != "on" && rest[0] != "off") {
			return invalidInvocation("sync auto requires on or off")
		}
		asJSON, err := parseOptionalJSON(rest[1:], "sync auto")
		if err != nil {
			return invocation{Kind: invocationInvalid}, err
		}
		return newSyncInvocation(syncInvocation{
			Action: syncAuto, Enabled: rest[0] == "on", JSON: asJSON,
		}), nil
	default:
		return invalidInvocation(fmt.Sprintf("unknown sync action %q", action))
	}
}

func validSyncAction(action string) bool {
	switch action {
	case "setup", "push", "pull", "now", "auto":
		return true
	default:
		return false
	}
}

func parseOptionalJSON(args []string, command string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--json" {
		return true, nil
	}
	return false, fmt.Errorf("usage: %s only accepts one --json", command)
}

func newSyncInvocation(sync syncInvocation) invocation {
	return invocation{Kind: invocationSync, JSON: sync.JSON, Sync: &sync}
}

// parseSSHInvocation は alias を ssh namespace の内側だけで解釈する。
// transport や将来の top-level command と同名でも、alias の意味は変わらない。
func parseSSHInvocation(args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(sshSubcommand), nil
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

// parseEngineFlags は `sshc engine` のオプションを解析する。
func parseEngineFlags(args []string) (invocation, error) {
	called := invocation{Kind: invocationEngine}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--replace":
			called.Replace = true
		case "--port":
			index++
			if index >= len(args) {
				return invalidInvocation("--port takes a number")
			}
			port, err := strconv.Atoi(args[index])
			// 無効な範囲と bind エラーを区別するため、ここで範囲を検証する。
			if err != nil || port < 1024 || port > 65535 {
				return invalidInvocation("--port takes a number between 1024 and 65535")
			}
			called.Port = port
		default:
			return invalidInvocation("engine does not take " + args[index])
		}
	}
	return called, nil
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

// usage は予約済みサブコマンドと引数を出力する。
func usage(out io.Writer) {
	fmt.Fprint(out, `usage:
  sshc                 open the UI for the running engine
  sshc engine          start the engine in the foreground
                       --port <n>  listen there instead of a random port
                       --replace   stop the running engine first, without asking
  sshc ssh [<alias>]   choose a host, or connect to one from ~/.ssh/config
                       --list      print every concrete Host alias
  sshc ssh <alias> --non-interactive -- <command>
                       run an SSH command without an interactive terminal
  sshc info <alias> [--json]
                       print the resolved SSH target without connecting
  sshc sync [--json]   print synchronization status from the running engine
  sshc sync setup      configure synchronization in an interactive terminal
  sshc sync push [--force] [--json]
  sshc sync pull [--force] [--json]
  sshc sync now [--json]
  sshc sync auto on|off [--json]
                       run or configure synchronization through the engine
  sshc terminal list [--json]
  sshc terminal show <session-id> [--json]
  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]
  sshc terminal send <session-id> --text <text> [--no-enter] [--json]
  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]
                       states: connecting, connected, reconnecting, exited,
                               agent-working, agent-attention, agent-ready, agent-ended
  sshc terminal create shell [--json]
  sshc terminal create ssh <alias> [--json]
  sshc terminal rename <session-id> <title> [--json]
  sshc terminal close <session-id> [--json]
                       inspect and control terminals owned by the running engine
  sshc serial [--json]
                       list serial devices
  sshc serial <device> [options]
                       connect interactively to a serial device
                       options: --baud N --data-bits 5..8 --parity none|odd|even|mark|space
                                --stop-bits 1|1.5|2 --flow none|rtscts|xonxoff
                                --dtr on|off --rts on|off --break D --encoding NAME
  sshc telnet <host>[:port] [options]
                       connect interactively with unencrypted Telnet
                       options: --connect-timeout D --terminal-type TYPE --encoding NAME
  sshc serial <device> [options] --non-interactive [automation] -- <text>
  sshc telnet <host>[:port] [options] --non-interactive [automation] -- <text>
                       send text and wait for --expect or --read-for
                       automation: --expect REGEX | --read-for D | --script FILE|-
                                   --timeout D --settle D --max-bytes N --line-ending MODE
                                   --require-output --json
                       encodings: utf-8, shift_jis, euc-jp, iso-2022-jp
  sshc open            print a one-time UI URL
  sshc status          print what the running engine is doing
                       --json      print it as JSON, for the shell
  sshc update          update an installation managed by Homebrew or install.sh
  sshc service install install and start a systemd user service on Linux
  sshc service status  print whether the managed service is active
  sshc service disable stop and remove the managed service
  sshc vault status    describe the running engine and vault
  sshc vault create    create and unlock a new vault
  sshc vault unlock    unlock the vault in the running engine
  sshc vault lock      lock the vault without closing SSH sessions
  sshc vault change-password
                       change the password of an unlocked vault
  sshc version         print the version, and what it was built for
  sshc help [<command> ...]
                       print all commands or help for one command

`)
}

func validHelpTopic(topic string) bool {
	switch topic {
	case engineSubcommand, sshSubcommand, infoSubcommand,
		syncSubcommand, "sync setup", "sync push", "sync pull", "sync now", "sync auto",
		terminalSubcommand, "terminal list", "terminal show", "terminal read", "terminal send",
		"terminal wait", "terminal create", "terminal rename", "terminal close",
		serialSubcommand, telnetSubcommand, OpenSubcommand, StatusSubcommand, updateSubcommand,
		serviceSubcommand, "service install", "service status", "service disable",
		vaultSubcommand, "vault status", "vault create", "vault unlock", "vault lock",
		"vault change-password", versionSubcommand:
		return true
	default:
		return false
	}
}

func usageFor(out io.Writer, topic string) {
	if topic == "" {
		usage(out)
		return
	}
	help := map[string]string{
		engineSubcommand: `usage:
  sshc engine [--port <n>] [--replace]

Start the engine in the foreground.
  --port <n>  listen on a port from 1024 to 65535
  --replace   stop the running engine first, without asking
`,
		sshSubcommand: `usage:
  sshc ssh
  sshc ssh --list
  sshc ssh <alias>
  sshc ssh <alias> --non-interactive -- <command>

Choose or connect to a Host alias from ~/.ssh/config.
`,
		infoSubcommand: `usage:
  sshc info <alias> [--json]

Print the resolved SSH target without connecting.
`,
		syncSubcommand: `usage:
  sshc sync [--json]
  sshc sync setup
  sshc sync push [--force] [--json]
  sshc sync pull [--force] [--json]
  sshc sync now [--json]
  sshc sync auto on|off [--json]
`,
		"sync setup": `usage:
  sshc sync setup

Configure synchronization in an interactive terminal.
`,
		"sync push": `usage:
  sshc sync push [--force] [--json]

Push local synchronization changes through the running engine.
`,
		"sync pull": `usage:
  sshc sync pull [--force] [--json]

Pull synchronization changes through the running engine.
`,
		"sync now": `usage:
  sshc sync now [--json]

Run one synchronization cycle through the running engine.
`,
		"sync auto": `usage:
  sshc sync auto on|off [--json]

Enable or disable automatic synchronization.
`,
		terminalSubcommand: `usage:
  sshc terminal list [--json]
  sshc terminal show <session-id> [--json]
  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]
  sshc terminal send <session-id> --text <text> [--no-enter] [--json]
  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]
  sshc terminal create shell [--json]
  sshc terminal create ssh <alias> [--json]
  sshc terminal rename <session-id> <title> [--json]
  sshc terminal close <session-id> [--json]
`,
		"terminal list": `usage:
  sshc terminal list [--json]

List terminals owned by the running engine.
`,
		"terminal show": `usage:
  sshc terminal show <session-id> [--json]

Show one terminal. The ID may be a unique lowercase hexadecimal prefix.
`,
		"terminal read": `usage:
  sshc terminal read <session-id> [--cursor N] [--limit N] [--json]

Read retained terminal output from a byte cursor.
`,
		"terminal send": `usage:
  sshc terminal send <session-id> --text <text> [--no-enter] [--json]

Send text to the current process generation. A carriage return is appended
unless --no-enter is set.
`,
		"terminal wait": `usage:
  sshc terminal wait <session-id> --for <state> [--timeout D] [--json]

States: connecting, connected, reconnecting, exited, agent-working,
agent-attention, agent-ready, agent-ended.
`,
		"terminal create": `usage:
  sshc terminal create shell [--json]
  sshc terminal create ssh <alias> [--json]

Create a local shell or SSH terminal in the running engine.
`,
		"terminal rename": `usage:
  sshc terminal rename <session-id> <title> [--json]

Set the title of a terminal owned by the running engine.
`,
		"terminal close": `usage:
  sshc terminal close <session-id> [--json]

Close a terminal owned by the running engine.
`,
		serialSubcommand: `usage:
  sshc serial [--json]
  sshc serial <device> [options]
  sshc serial <device> [options] --non-interactive [automation] -- <text>

Options: --baud N --data-bits 5..8 --parity none|odd|even|mark|space
         --stop-bits 1|1.5|2 --flow none|rtscts|xonxoff
         --dtr on|off --rts on|off --break D --encoding NAME
Automation: --expect REGEX | --read-for D | --script FILE|-
            --timeout D --settle D --max-bytes N --line-ending MODE
            --require-output --json
`,
		telnetSubcommand: `usage:
  sshc telnet <host>[:port] [options]
  sshc telnet <host>[:port] [options] --non-interactive [automation] -- <text>

Options: --connect-timeout D --terminal-type TYPE --encoding NAME
Automation: --expect REGEX | --read-for D | --script FILE|-
            --timeout D --settle D --max-bytes N --line-ending MODE
            --require-output --json
`,
		OpenSubcommand: `usage:
  sshc open

Print a one-time UI URL for the running engine.
`,
		StatusSubcommand: `usage:
  sshc status [--json]

Print what the running engine is doing.
`,
		updateSubcommand: `usage:
  sshc update

Update an installation managed by Homebrew or install.sh.
`,
		serviceSubcommand: `usage:
  sshc service install
  sshc service status
  sshc service disable

Manage the sshc engine as a systemd user service on Linux.
`,
		"service install": `usage:
  sshc service install

Install, enable, and start the sshc systemd user service on Linux.
`,
		"service status": `usage:
  sshc service status

Print whether the sshc-managed systemd user service is active.
`,
		"service disable": `usage:
  sshc service disable

Stop, disable, and remove the sshc-managed systemd user service.
`,
		vaultSubcommand: `usage:
  sshc vault status
  sshc vault create
  sshc vault unlock
  sshc vault lock
  sshc vault change-password
`,
		"vault status": `usage:
  sshc vault status

Describe the running engine and Vault.
`,
		"vault create": `usage:
  sshc vault create

Create and unlock a new Vault.
`,
		"vault unlock": `usage:
  sshc vault unlock

Unlock the Vault in the running engine.
`,
		"vault lock": `usage:
  sshc vault lock

Lock the Vault without closing SSH sessions.
`,
		"vault change-password": `usage:
  sshc vault change-password

Change the password of an unlocked Vault.
`,
		versionSubcommand: `usage:
  sshc version

Print the version and target operating system/architecture.
`,
	}
	fmt.Fprint(out, help[topic])
}
