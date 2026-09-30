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
		os.Exit(exitUsage)
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
			os.Exit(exitFailure)
		}
		os.Exit(0)
	}
	if called.Kind == invocationTransport || called.Kind == invocationRunTransport {
		ctx, stopSignals := notifySignals(context.Background())
		defer stopSignals()
		code := runTransportInvocation(ctx, *called.Transport, os.Stdin, os.Stdout, os.Stderr)
		os.Exit(transportExitCode(called.Kind, code, context.Cause(ctx)))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
		os.Exit(exitFailure)
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
	case invocationOpenInBrowser:
		// 引数なしでは engine の UI URL を取得し、ブラウザで開く。engine は起動しない。
		return runOpen(ctx, systemCommandEnvironment(home, client), true)
	case invocationEngine:
		// engine は stdin を読み取らない。
		return runEngine(ctx, home,
			engineOptions{Port: called.Port, Replace: called.Replace},
			os.Stdin, os.Stdout, os.Stderr)
	case invocationConnect:
		// SIGTERM と SIGHUP も取り消しにする。既定のまま落ちると、raw にした
		// ターミナルを戻す defer を通らない。
		connectCtx, stopSignals := notifySignals(ctx)
		defer stopSignals()
		return runConnect(connectCtx, called.Args[0], systemCommandEnvironment(home, client))
	case invocationRun:
		return runRemote(ctx, called.Args[0], remoteCommand(called.Args[1:]), systemCommandEnvironment(home, client))
	case invocationChoose:
		query := ""
		if len(called.Args) != 0 {
			query = called.Args[0]
		}
		// 選択画面は代替画面とカーソルの非表示を、戻す defer を通らずに落ちると
		// 残す。SIGTERM と SIGHUP も取り消しにして、その defer まで戻る。
		chooseCtx, stopSignals := notifySignals(ctx)
		defer stopSignals()
		alias, err := chooseTUIHost(chooseCtx, tuiPicker{
			home: home, initialQuery: query, input: os.Stdin, output: os.Stdout, stderr: os.Stderr,
		})
		if err != nil {
			if errors.Is(err, errTUIClosed) {
				return 0
			}
			if code, stopped := interactiveStopExitCode(chooseCtx); stopped {
				return code
			}
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			return exitFailure
		}
		return runConnect(chooseCtx, alias, systemCommandEnvironment(home, client))
	case invocationList:
		return runList(home, os.Stdout, os.Stderr)
	case invocationInfo:
		return runInfo(called.Args[0], home, called.JSON, os.Stdout, os.Stderr)
	case invocationOpen:
		return runOpen(ctx, systemCommandEnvironment(home, client), false)
	case invocationStatus:
		return runStatus(ctx, systemCommandEnvironment(home, client), called.JSON)
	case invocationUpdate:
		updateCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runUpdate(updateCtx, updateRun{
			current: version, yes: called.Yes, home: home, stdout: os.Stdout, stderr: os.Stderr,
			dependencies: defaultUpdateDependencies(),
		})
	case invocationService:
		serviceCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runService(serviceCtx, serviceRun{
			action: called.Args[0], yes: called.Yes, home: home, stdout: os.Stdout, stderr: os.Stderr,
			dependencies: defaultServiceDependencies(),
		})
	case invocationOTP:
		otpCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runOTP(otpCtx, *called.OTP, systemCommandEnvironment(home, client))
	case invocationVPN:
		vpnCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runVPN(vpnCtx, *called.VPN, systemCommandEnvironment(home, client))
	case invocationVault:
		// password 読み取り中と loopback request 中の Ctrl-C を public 130 にする。
		// SIGTERM と SIGHUP も同じく取り消しにする。既定のまま落ちると、エコーを
		// 止めたターミナルを戻す defer を通らない。engine の ownership signal は
		// runEngine が別に持つため、ここでは利用者が起動する短命な Vault command
		// だけを対象にする。
		vaultCtx, stopSignals := notifySignals(ctx)
		defer stopSignals()
		return runVault(vaultCtx, called.Args[0], systemCommandEnvironment(home, vaultCommandClient(client)))
	case invocationSync:
		syncCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runSync(syncCtx, *called.Sync, systemCommandEnvironment(home, client))
	case invocationTerminal:
		terminalCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runTerminal(terminalCtx, *called.Terminal, systemCommandEnvironment(home, client))
	case invocationSFTP:
		sftpCtx, cancel := signal.NotifyContext(ctx, os.Interrupt)
		defer cancel()
		return runSFTP(sftpCtx, *called.SFTP, systemCommandEnvironment(home, client))
	default:
		fmt.Fprintln(os.Stderr, "sshc: invalid invocation")
		return exitUsage
	}
}
