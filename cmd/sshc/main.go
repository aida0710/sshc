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

// printVersion は version、OS、architecture を 1 行で出力する。
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
		usageFor(os.Stdout, called.HelpTopic)
		os.Exit(0)
	}
	if called.Kind == invocationVersion {
		printVersion(os.Stdout)
		os.Exit(0)
	}
	if called.Kind == invocationCompletion {
		if err := writeCompletion(os.Stdout, called.Args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if called.Kind == invocationTransport || called.Kind == invocationRunTransport {
		ctx, stopSignals := notifySignals(context.Background())
		defer stopSignals()
		code := runTransportInvocation(ctx, *called.Transport, os.Stdin, os.Stdout, os.Stderr)
		if code == transportInterruptedExit && errors.Is(context.Cause(ctx), errEngineTerminated) {
			code = 0
		}
		os.Exit(code)
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
		// 引数なしでは engine の UI URL を取得し、ブラウザで開く。engine は起動しない。
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr, true)
	case invocationEngine:
		// engine は stdin を読み取らない。
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
	case invocationInfo:
		return runInfo(called.Args[0], home, called.JSON, os.Stdout, os.Stderr)
	case invocationOpen:
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr, false)
	case invocationStatus:
		return runStatus(ctx, app.HandoffDir(home), client, called.JSON, os.Stdout, os.Stderr)
	case invocationUpdate:
		updateCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runUpdate(updateCtx, version, os.Stdout, os.Stderr, defaultUpdateDependencies())
	case invocationService:
		serviceCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runService(serviceCtx, called.Args[0], home, os.Stdout, os.Stderr, defaultServiceDependencies())
	case invocationVault:
		// password 読み取り中と loopback request 中の Ctrl-C を public 130 にする。
		// engine の ownership signal は runEngine が別に持つため、ここでは利用者が
		// 起動する短命な Vault command だけを対象にする。
		vaultCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runVault(vaultCtx, called.Args[0], app.HandoffDir(home), vaultCommandClient(client),
			os.Stdin, os.Stdout, os.Stderr, systemPasswordTerminal{})
	case invocationSync:
		syncCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runSync(syncCtx, *called.Sync, app.HandoffDir(home), client,
			os.Stdin, os.Stdout, os.Stderr, systemPasswordTerminal{})
	case invocationTerminal:
		terminalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runTerminal(terminalCtx, *called.Terminal, app.HandoffDir(home), client,
			os.Stdout, os.Stderr)
	default:
		fmt.Fprintln(os.Stderr, "sshc: invalid invocation")
		return 2
	}
}
