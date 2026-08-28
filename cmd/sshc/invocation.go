package main

import (
	"fmt"
	"io"
	"strconv"
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
	invocationHelp
	invocationVersion
	invocationTransport
	invocationRunTransport
)

type invocation struct {
	Kind invocationKind
	Args []string
	// Port は `sshc engine --port N` の待受ポートで、0 は未指定を表す。
	Port int
	// Replace は既存の engine を確認なしで停止して置き換える。
	Replace bool
	// JSON は `sshc status --json` の機械可読出力を選択する。
	JSON      bool
	Transport *transportInvocation
}

const (
	engineSubcommand  = "engine"
	vaultSubcommand   = "vault"
	runSubcommand     = "run"
	helpSubcommand    = "help"
	versionSubcommand = "version"
	updateSubcommand  = "update"
	StatusSubcommand  = "status"
	serialSubcommand  = "serial"
	telnetSubcommand  = "telnet"
	sshSubcommand     = "ssh"
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
		return parseEngineFlags(args)
	case ConnectSubcommand:
		if len(args) > 1 {
			return invalidInvocation("connect takes at most one search term")
		}
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return invocation{Kind: invocationHelp}, nil
		}
		return invocation{Kind: invocationChoose, Args: copyInvocationArgs(args)}, nil
	case runSubcommand:
		if len(args) > 0 && args[0] == serialSubcommand {
			return parseTransportInvocation(transportSerial, args[1:], true)
		}
		if len(args) > 0 && args[0] == telnetSubcommand {
			return parseTransportInvocation(transportTelnet, args[1:], true)
		}
		if len(args) > 0 && args[0] == sshSubcommand {
			if len(args) < 4 || args[2] != "--" {
				return invalidInvocation("run ssh requires an alias, --, and a command")
			}
			return invocation{Kind: invocationRun, Args: append([]string{args[1]}, args[3:]...)}, nil
		}
		// 先頭引数を接続先、残りをリモートコマンドとして扱う。
		if len(args) < 2 {
			return invalidInvocation("run requires an alias and a command")
		}
		return invocation{Kind: invocationRun, Args: copyInvocationArgs(args)}, nil
	case serialSubcommand:
		return parseTransportInvocation(transportSerial, args, false)
	case telnetSubcommand:
		return parseTransportInvocation(transportTelnet, args, false)
	case sshSubcommand:
		if len(args) != 1 {
			return invalidInvocation("ssh requires one alias")
		}
		return invocation{Kind: invocationConnect, Args: copyInvocationArgs(args)}, nil
	case ListSubcommand:
		return noArguments(invocationList, word, args)
	case OpenSubcommand:
		return noArguments(invocationOpen, word, args)
	case StatusSubcommand:
		if len(args) == 1 && args[0] == "--json" {
			return invocation{Kind: invocationStatus, JSON: true}, nil
		}
		return noArguments(invocationStatus, word, args)
	case updateSubcommand:
		return noArguments(invocationUpdate, word, args)
	case vaultSubcommand:
		if len(args) != 1 {
			return invalidInvocation("vault requires one action")
		}
		switch args[0] {
		case "status", "create", "unlock", "lock", "change-password":
			return invocation{Kind: invocationVault, Args: copyInvocationArgs(args)}, nil
		default:
			return invalidInvocation(fmt.Sprintf("unknown vault action %q", args[0]))
		}
	case helpSubcommand, "-h", "--help":
		return noArguments(invocationHelp, word, args)
	// 旗も語も、同じところへ着く。`sshc version` が正式だが、`--version` は
	// 誰もが最初に打つ形である。受けないと、入れた直後の一行目が usage と
	// 終了コード 2 になる。実際 docs/release-install.md はそれを案内していた。
	case versionSubcommand, "-v", "--version":
		return noArguments(invocationVersion, word, args)
	}

	if word == "" || word[0] == '-' {
		return invalidInvocation(fmt.Sprintf("unknown command %q", word))
	}
	if len(args) != 0 {
		return invalidInvocation("an SSH alias takes no extra arguments")
	}
	return invocation{Kind: invocationConnect, Args: []string{word}}, nil
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
  sshc <alias>         connect to a host from ~/.ssh/config in this terminal
  sshc run <alias> ... run a non-interactive command on a host
  sshc ssh <alias>     connect to an alias named like a reserved command
  sshc run ssh <alias> -- ...
                       explicitly run an SSH command
  sshc serial list [--json]
                       list serial devices
  sshc serial <device> [options]
                       connect interactively to a serial device
                       options: --baud N --data-bits 5..8 --parity none|odd|even|mark|space
                                --stop-bits 1|1.5|2 --flow none|rtscts|xonxoff
                                --dtr on|off --rts on|off --break D --encoding NAME
  sshc telnet <host>[:port] [options]
                       connect interactively with unencrypted Telnet
                       options: --connect-timeout D --terminal-type TYPE --encoding NAME
  sshc run serial <device> [options] -- <text>
  sshc run telnet <host>[:port] [options] -- <text>
                       send text and wait for --expect or --read-for
                       automation: --expect REGEX | --read-for D | --script FILE|-
                                   --timeout D --settle D --max-bytes N --line-ending MODE --json
                       encodings: utf-8, shift_jis, euc-jp, iso-2022-jp
  sshc connect [text]  choose a host in this terminal, then connect
  sshc list            print every concrete Host alias, one per line
  sshc open            print a one-time UI URL
  sshc status          print what the running engine is doing
                       --json      print it as JSON, for the shell
  sshc update          update an installation managed by Homebrew or install.sh
  sshc vault status    describe the running engine and vault
  sshc vault create    create and unlock a new vault
  sshc vault unlock    unlock the vault in the running engine
  sshc vault lock      lock the vault without closing SSH sessions
  sshc vault change-password
                       change the password of an unlocked vault
  sshc version         print the version, and what it was built for
  sshc help            print this

`)
}
