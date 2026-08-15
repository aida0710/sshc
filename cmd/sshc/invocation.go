package main

import (
	"fmt"
	"io"
)

type invocationKind uint8

const (
	invocationInvalid invocationKind = iota
	invocationDesktop
	invocationEngine
	invocationHeadless
	invocationConnect
	invocationChoose
	invocationList
	invocationOpen
	invocationStatus
	invocationVault
	invocationHelp
)

type invocation struct {
	Kind invocationKind
	Args []string
}

const (
	engineSubcommand   = "engine"
	headlessSubcommand = "headless"
	vaultSubcommand    = "vault"
	helpSubcommand     = "help"
	StatusSubcommand   = "status"
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
		return noArguments(invocationEngine, word, args)
	case headlessSubcommand:
		return noArguments(invocationHeadless, word, args)
	case ConnectSubcommand:
		if len(args) > 1 {
			return invalidInvocation("connect takes at most one search term")
		}
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			return invocation{Kind: invocationHelp}, nil
		}
		return invocation{Kind: invocationChoose, Args: copyInvocationArgs(args)}, nil
	case ListSubcommand:
		return noArguments(invocationList, word, args)
	case OpenSubcommand:
		return noArguments(invocationOpen, word, args)
	case StatusSubcommand:
		return noArguments(invocationStatus, word, args)
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
	}

	if word == "" || word[0] == '-' {
		return invalidInvocation(fmt.Sprintf("unknown command %q", word))
	}
	if len(args) != 0 {
		return invalidInvocation("an SSH alias takes no extra arguments")
	}
	return invocation{Kind: invocationConnect, Args: []string{word}}, nil
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

// usage は、alias より先に読む予約語を全て示す。
//
// `sshc engine` は Electron が子 engine の lifetime を所有するためだけの入口であり、
// 普通の利用者が直接実行するものではない。この理由を usage に出すことで、内部用の
// 語が通常の headless 起動と同じものに見えるのを避ける。
func usage(out io.Writer) {
	fmt.Fprint(out, `usage:
  sshc                 launch or focus the desktop application
  sshc engine          internal: run the engine owned by Electron
  sshc headless        run a foreground engine for terminals and supervisors
  sshc <alias>         connect to a host from ~/.ssh/config in this terminal
  sshc connect [text]  choose a host in this terminal, then connect
  sshc list            print every concrete Host alias, one per line
  sshc open            print a new way into the UI
  sshc status          print the engine's status as JSON, for the shell
  sshc vault <action>  manage the running engine's vault
  sshc help            print this

`)
}
