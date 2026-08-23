package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"

	"sshc/internal/app"
)

var version = "dev"

// printVersion は、この実体が何であるかを 1 行で言う。
//
// **OS と アーキテクチャを一緒に出す。** 入れ方が増えた——brew、install.sh、
// 束の中、`make install` ——ので、「入ったが動かない」の相談で最初に要るのは
// 版よりも「どれをどの機械に入れたのか」である。Rosetta の下で走る amd64 の
// 実体は、それを言われるまで見分けがつかない。
func printVersion(out io.Writer) {
	fmt.Fprintf(out, "sshc %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
}

func main() {
	called, err := parseInvocation(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
		usage(os.Stderr)
		os.Exit(2)
	}
	if called.Kind == invocationHelp {
		usage(os.Stdout)
		os.Exit(0)
	}
	if called.Kind == invocationVersion {
		printVersion(os.Stdout)
		os.Exit(0)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
		os.Exit(1)
	}
	client := &http.Client{Timeout: connectTimeout}
	os.Exit(dispatchInvocation(called, home, client))
}

// dispatchInvocation は、解析済みの呼び出しを、その owner ごとの処理へ渡す。
// parser が形を保証するため、ここでは argv を読み直さない。将来 engine の ownership
// transport を入れても、CLI の引数解釈へ戻らずこの境界だけを置き換えられる。
func dispatchInvocation(called invocation, home string, client *http.Client) int {
	ctx := context.Background()
	switch called.Kind {
	case invocationDesktop:
		// 裸の `sshc` は入口を刷り、開ける画面があれば開く。**engine は起こさない。**
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr, true)
	case invocationEngine:
		// **engine は stdin を読まない。** 端末で走るものが読み始めれば、
		// 打った人の入力を吸い込む。
		return runEngine(ctx, home,
			engineOptions{Port: called.Port, Replace: called.Replace},
			os.Stdin, os.Stdout, os.Stderr)
	case invocationConnect:
		return runConnect(ctx, called.Args[0], home, app.HandoffDir(home), client, os.Stdin, os.Stdout, os.Stderr)
	case invocationRun:
		return runRemote(ctx, called.Args[0], remoteCommand(called.Args[1:]), home,
			app.HandoffDir(home), client,
			os.Stdin, os.Stdout, os.Stderr)
	case invocationChoose:
		query := ""
		if len(called.Args) != 0 {
			query = called.Args[0]
		}
		alias, err := chooseTUIHost(home, query, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			if errors.Is(err, errTUIClosed) {
				return 0
			}
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			return 1
		}
		return runConnect(ctx, alias, home, app.HandoffDir(home), client, os.Stdin, os.Stdout, os.Stderr)
	case invocationList:
		return runList(home, os.Stdout, os.Stderr)
	case invocationOpen:
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr, false)
	case invocationStatus:
		return runStatus(ctx, app.HandoffDir(home), client, called.JSON, os.Stdout, os.Stderr)
	case invocationVault:
		// password 読み取り中と loopback request 中の Ctrl-C を public 130 にする。
		// engine の ownership signal は runEngine が別に持つため、ここでは利用者が
		// 起動する短命な Vault command だけを対象にする。
		vaultCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runVault(vaultCtx, called.Args[0], app.HandoffDir(home), vaultCommandClient(client),
			os.Stdin, os.Stdout, os.Stderr, systemPasswordTerminal{})
	default:
		fmt.Fprintln(os.Stderr, "sshc: invalid invocation")
		return 2
	}
}
