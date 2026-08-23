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
	invocationHelp
	invocationVersion
)

type invocation struct {
	Kind invocationKind
	Args []string
	// Port は `sshc engine --port N` で選ばれた受け口である。0 は「選んでいない」。
	//
	// **旗は保存された設定より強い。** その場で決めた人が居るのに、書いてある方を
	// 使えば、打った番号がどこにも効かない。
	Port int
	// Replace は、走っている engine を止めてから起こしてよいという合図である。
	//
	// **端末で訊く道とは別に、これが要る。** 手順の中や supervisor の下では
	// 訊く相手が居ない——そこでは、書いた人が先に答えを決めておくしかない。
	Replace bool
}

const (
	engineSubcommand  = "engine"
	vaultSubcommand   = "vault"
	runSubcommand     = "run"
	helpSubcommand    = "help"
	versionSubcommand = "version"
	StatusSubcommand  = "status"
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
		// **接続先とコマンドを、ひとつの語で兼ねさせない。** 先頭が接続先で、
		// 残りがすべてコマンドである。境目を推測する余地を作らない。
		if len(args) < 2 {
			return invalidInvocation("run requires an alias and a command")
		}
		return invocation{Kind: invocationRun, Args: copyInvocationArgs(args)}, nil
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
	// **旗も語も、同じところへ着く。** `sshc version` が正式だが、`--version` は
	// 誰もが最初に打つ形である——受けないと、入れた直後の一行目が usage と
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

// parseEngineFlags は `sshc engine` の旗を読む。
//
// **旗は 2 つだけである。** 増やすたびに、engine を起こす方法が増える——起こし方が
// 一つであることは、この道具の形そのものである。
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
			// **範囲もここで断る。** 通してしまうと、断るのは bind の失敗になり、
			// 打った人には「使えない番号」と「埋まっている番号」が同じに見える。
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

// usage は、alias より先に読む予約語を全て示す。
//
// **`sshc engine` は前面で走り続ける。** 生かしておくのは人であり、この道具では
// ない——tmux でも systemd でも、その計算機でプロセスを持つ作法に任せる。裸の
// `sshc` はそれを起こさず、走っているものへの入口を刷るだけである。
func usage(out io.Writer) {
	fmt.Fprint(out, `usage:
  sshc                 print a way into the running engine, and open it
  sshc engine          run the engine in the foreground; keep it alive yourself
                       --port <n>  listen there instead of a random port
                       --replace   stop the running engine first, without asking
  sshc <alias>         connect to a host from ~/.ssh/config in this terminal
  sshc run <alias> ... run one command on a host and print what it wrote
  sshc connect [text]  choose a host in this terminal, then connect
  sshc list            print every concrete Host alias, one per line
  sshc open            print a new way into the UI
  sshc status          print the engine's status as JSON, for the shell
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
