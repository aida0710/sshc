package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/selfupdate"
	"sshc/internal/ui"
)

var version = "dev"

// engineBusyExit は、Electron が所有する engine が既存 engine の lock を取れなかった
// ときの終了コードである。外殻は自分の子を殺せても他人の engine は殺せないため、
// 入口を渡さずこの番号で ownership の衝突を知らせる。
const engineBusyExit = 3

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
		if launchApp(ctx) {
			return 0
		}
		fmt.Fprintln(os.Stderr, "sshc: could not launch the desktop application")
		return 1
	case invocationEngine:
		return runEngine(home, client, true)
	case invocationHeadless:
		return runEngine(home, client, false)
	case invocationConnect:
		return runConnect(ctx, called.Args[0], home, app.HandoffDir(home), client, os.Stdin, os.Stdout, os.Stderr)
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
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr)
	case invocationStatus:
		return runStatus(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr)
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

// runEngine は、当面 `engine` と `headless` が共有する既存の foreground runner である。
// ownership transport はまだ導入しないが、Electron の子だけは別 engine の入口を成功として
// 受け取れないため、busy 時の終了コードだけを owner 境界として残す。
func runEngine(home string, client *http.Client, electronOwns bool) int {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// **アプリが消えたらエンジンも消える。** 親を見張るのは、通常の終了
	// 経路（親が kill する）が働かなかったときのためである。起こしてくれた
	// 親をいま控えるのは、孤児の引き取り手が init とは限らないからである。
	go watchParent(ctx, os.Getppid, os.Getppid(), parentTick, stop)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	assets, err := ui.FS()
	if err != nil {
		logger.Error("load embedded UI", "error", err)
		os.Exit(1)
	}

	// **エンジンの寿命ぶんロックを握る。** ここへ来るのは parser が engine
	// owner と明示した呼び出しだけである。裸の起動を紛れ込ませないことで、誰が
	// engine の寿命を引き受けるかを dispatch の外へ漏らさない。
	//
	// 握れなければ 1 台目が生きている。**そのときは走っている方の入口を出して
	// 終わる。** 打った人にとっては「入口が出る」という自然な結果であり、
	// handoff を上書きする 2 台目はどこにも生まれない。
	release, err := lockEngineStart(app.HandoffDir(home))
	switch {
	case errors.Is(err, errEngineRunning) && electronOwns:
		return engineBusyExit
	case errors.Is(err, errEngineRunning):
		// **勝った方が handoff を書き終えるまで待つ。** ロックは listener より
		// 先に取れるので、ほぼ同時に打たれた 2 つのうち負けた方がその隙に
		// 読むと `sshc: not running` で 1 になる——理由の無い失敗に見える。
		waitForHandoff(ctx, app.HandoffDir(home))
		return runOpen(ctx, app.HandoffDir(home), client, os.Stdout, os.Stderr)
	case err != nil:
		logger.Error("take the engine lock", "error", err)
		return 1
	}
	defer release()

	parts := newPlatformParts()
	owner := handoff.OwnerHeadless
	if electronOwns {
		// Electron が子を終了させる desktop と、端末・supervisor が寿命を持つ
		// headless を文書でも分ける。bool をここから先へ漏らさないためである。
		owner = handoff.OwnerDesktop
	}

	announce := func(entrance string) error {
		_, err := fmt.Fprintln(os.Stdout, entrance)
		return err
	}

	dependencies := app.Dependencies{
		Random:   rand.Reader,
		Announce: announce,
		// このアプリケーションが自分自身以外のホストに接触する唯一の場所であり、
		// 誰かが求めたときにだけ行う。何も取得せず、何も置き換えない。
		// 新しいバージョンが公開されているかを報告するだけである。
		Updates: &selfupdate.Checker{
			API:  "https://api.github.com/repos/aida0710/sshc/releases/latest",
			HTTP: &http.Client{Timeout: 30 * time.Second},
		},
		Listen:    net.Listen,
		UI:        assets,
		Logger:    logger,
		Home:      home,
		Owner:     owner,
		PID:       os.Getpid(),
		Toolchain: parts.Toolchain,
		KeyAgent:  parts.KeyAgent,
		Lookup:    os.LookupEnv,
		Environ:   os.Environ,
	}
	if err := app.Run(ctx, dependencies, version); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("sshc stopped", "error", err)
		return 1
	}
	return 0
}
