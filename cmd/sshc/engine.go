package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"golang.org/x/term"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/releasecheck"
	"sshc/internal/ui"
)

// engineDependencies は、この runner のインターフェースである。
//
// 可変のパッケージ変数にしない。差し替え可能なグローバルを置くと、
// 並行して走るパッケージのテストが互いの依存関係を書き換えてしまう。
type engineDependencies struct {
	acquire         func(string) (func() error, error)
	runApp          func(context.Context, app.Dependencies, string) error
	openBrowser     func(string) bool
	shutdownTimeout time.Duration
}

func defaultEngineDependencies() engineDependencies {
	return engineDependencies{
		acquire:     lockEngineStart,
		runApp:      app.Run,
		openBrowser: openInBrowser,
	}
}

// engineOptions は、`sshc engine` の旗が決めたことである。
type engineOptions struct {
	// Port は 0 なら「決めていない」。保存された設定より強い。
	Port int
	// Replace は、走っている engine を訊かずに止めてよいという合図である。
	Replace bool
}

func runEngine(
	ctx context.Context,
	paths userPaths,
	options engineOptions,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	return runEngineWithDependencies(ctx, paths, options, stdin, stdout, stderr, defaultEngineDependencies())
}

func runEngineWithDependencies(
	ctx context.Context,
	paths userPaths,
	options engineOptions,
	stdin io.Reader,
	stdout, stderr io.Writer,
	dependencies engineDependencies,
) int {
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	signalCtx, stopSignals := notifySignals(ctx)
	defer stopSignals()

	// 所有権はロックより先である。持ち主が既に居なくなっていたなら、
	// ロックを取ってはならない。取れば、誰も待っていないエンジンが 1 台分の
	// 席を占める。
	release, err := dependencies.acquire(paths.stateDir)
	if errors.Is(err, errEngineRunning) {
		// 2 台目は立てない。ただし、どこで起動したか分からない engine を探して
		// 回らずに畳めるよう、走っているものを止める道は用意する。
		taken, takeErr := replaceRunningEngine(signalCtx, paths.stateDir, options, stdin, stdout, stderr, dependencies.acquire)
		if takeErr != nil {
			if signalCtx.Err() != nil {
				return exitForCause(context.Cause(signalCtx))
			}
			// 断ったことは、もう綴ってある。ここで重ねると同じ話が二度出る。
			if !errors.Is(takeErr, errAlreadyRunning) {
				fmt.Fprintf(stderr, "sshc: %v\n", takeErr)
			}
			return exitFailure
		}
		release, err = taken, nil
	}
	if err != nil {
		logger.Error("take the engine lock", "error", err)
		return exitFailure
	}
	if signalCtx.Err() != nil {
		if releaseErr := release(); releaseErr != nil {
			logger.Error("release the engine lock", "error", releaseErr)
			return exitFailure
		}
		return exitForCause(context.Cause(signalCtx))
	}

	code := runEngineApp(signalCtx, paths.home, options, stdout, logger, dependencies)

	// ロックを手放すのは最後である。これより後に状態を変えるものは何も無い。
	if err := release(); err != nil {
		logger.Error("release the engine lock", "error", err)
		return exitFailure
	}
	return code
}

// runEngineApp は、アプリケーションを走らせ、終わった理由を終了コードへ写す。
func runEngineApp(
	ctx context.Context,
	home string,
	options engineOptions,
	stdout io.Writer,
	logger *slog.Logger,
	dependencies engineDependencies,
) int {
	assets, err := ui.FS()
	if err != nil {
		logger.Error("load embedded UI", "error", err)
		return exitFailure
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go func() {
		select {
		case <-ctx.Done():
			cancel(context.Cause(ctx))
		case <-runCtx.Done():
		}
	}()

	parts := newPlatformParts()
	updates := &releasecheck.Checker{
		API:  latestReleaseAPI,
		HTTP: &http.Client{Timeout: releaseCheckTimeout},
	}
	announce := announceReadiness(stdout)
	dependencyValues := app.Dependencies{
		Random:      rand.Reader,
		Port:        options.Port,
		DefaultPort: app.DefaultPort,
		Announce: func(readiness app.Readiness) error {
			// HTTP受付開始を先に知らせる。最新バージョンの確認が遅い／失敗する場合もengineは
			// 既に利用でき、その失敗で停止させない。
			if err := announce(readiness); err != nil {
				return err
			}
			if readiness.BrowserRegistrationRequired && terminalOutput(stdout) && dependencies.openBrowser != nil {
				if !dependencies.openBrowser(readiness.Entrance) {
					if _, err := fmt.Fprintln(stdout, "sshc: open the UI with `sshc`"); err != nil {
						return err
					}
				}
			}
			reportAvailableUpdate(runCtx, updates, version, stdout, logger)
			return nil
		},
		// 起動通知とWeb UIの手動確認は同じrelease判定を使用する。
		Updates:         updates,
		Listen:          net.Listen,
		UI:              assets,
		Logger:          logger,
		Home:            home,
		Owner:           handoff.OwnerEngine,
		PID:             os.Getpid(),
		Toolchain:       parts.Toolchain,
		KeyAgent:        parts.KeyAgent,
		Lookup:          os.LookupEnv,
		Environ:         os.Environ,
		ShutdownTimeout: dependencies.shutdownTimeout,
	}

	runErr := dependencies.runApp(runCtx, dependencyValues, version)
	cause := context.Cause(runCtx)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		logger.Error("sshc stopped", "error", runErr)
		return exitFailure
	}
	return exitForCause(cause)
}

func terminalOutput(output io.Writer) bool {
	file, ok := output.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

const (
	// releaseCheckTimeout は、engine が最新リリースを GitHub に 1 回尋ねる HTTP の
	// 上限である。Web UI の手動確認も同じ Checker を使うので、応答しない GitHub の
	// 前で画面の利用者を長く待たせない長さにする。
	releaseCheckTimeout = 3 * time.Second
	// startupUpdateCheckTimeout は、起動時の確認の上限である。確認は受付開始の通知
	// （Announce）の中で行い、その間 app.Run は停止の要求や Serve の失敗を待つところ
	// へ進めない。releaseCheckTimeout より長くしても、先に HTTP の上限で切れるので効かない。
	startupUpdateCheckTimeout = releaseCheckTimeout
)

// reportAvailableUpdate はengineが受付を始めた直後に一度だけ確認し、新しいバージョンがある
// 場合だけ通知する。ネットワーク障害や出力失敗はengineの成否へ影響させない。
func reportAvailableUpdate(ctx context.Context, checker *releasecheck.Checker, current string, out io.Writer, logger *slog.Logger) {
	checkCtx, cancel := context.WithTimeout(ctx, startupUpdateCheckTimeout)
	defer cancel()
	latest, err := checker.Latest(checkCtx)
	if err != nil {
		if logger != nil {
			logger.Debug("check for an sshc update", "error", err)
		}
		return
	}
	if !releasecheck.Newer(current, latest.Version) || checkCtx.Err() != nil {
		return
	}
	if _, err := fmt.Fprint(out, availableUpdateNotice(latest.Version, runtime.GOOS)); err != nil && logger != nil {
		logger.Debug("announce an sshc update", "error", err)
	}
}

// exitForCause は、走行が終わった理由ひとつを終了コードへ写す。Ctrl-C だけを
// exitInterrupted にし、正常終了、SIGTERM、呼び出し側の取り消しは 0 にする。
func exitForCause(cause error) int {
	if errors.Is(cause, errInterrupted) {
		return exitInterrupted
	}
	return 0
}

// announceReadiness は、受付が始まったことを伝える。
//
// アクセス URLはここに出さない。出せば、ログにも端末にもワンタイムの資格情報が
// 残る。アクセス URLは `sshc` が求めたときに 1 つずつ発行される。
func announceReadiness(stdout io.Writer) func(app.Readiness) error {
	return func(readiness app.Readiness) error {
		_, err := fmt.Fprintln(stdout, readinessMessage(readiness))
		return err
	}
}

// readinessMessage は、受付を始めた engine の Vault について、利用者が次にすることを返す。
// パスワードなしの Vault は起動時にロックが解除されているので、解除を求めない。
func readinessMessage(readiness app.Readiness) string {
	switch {
	case !readiness.VaultExists:
		return "sshc: create the password vault with `sshc vault create`"
	case !readiness.VaultUnlocked:
		return "sshc: unlock the password vault with `sshc vault unlock`"
	default:
		return "sshc: engine ready; vault is unlocked"
	}
}
