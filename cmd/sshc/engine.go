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
	"os/signal"
	"time"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/selfupdate"
	"sshc/internal/ui"
)

// engineMode は、エンジンの寿命を誰が引き受けるかである。
//
// **bool にしない。** どちらの owner かは、終了コードも、入口の出し方も、
// stdin を見るかどうかも変える。真偽値ひとつでそれを運ぶと、意味が呼び出し側の
// 記憶に残るだけになる。
type engineMode uint8

const (
	// engineDesktop は Electron が持つ内部のエンジンである。
	engineDesktop engineMode = iota + 1
	// engineHeadless は、端末や supervisor が持つ公開のエンジンである。
	engineHeadless
)

// 終了の理由。context.WithCancelCause でこれを運び、終了コードを決める。
var (
	errEngineInterrupted = errors.New("engine interrupted")
	errEngineTerminated  = errors.New("engine terminated")
)

// engineDependencies は、この runner の継ぎ目である。
//
// **可変のパッケージ変数にしない。** 差し替え可能なグローバルを置くと、
// 並行して走るパッケージのテストが互いの継ぎ目を踏む。
type engineDependencies struct {
	acquire          func(string) (func() error, error)
	ownershipMonitor func(io.Reader) (ownershipMonitor, error)
	runApp           func(context.Context, app.Dependencies, string) error
	shutdownTimeout  time.Duration
}

func defaultEngineDependencies() engineDependencies {
	return engineDependencies{
		acquire:          lockEngineStart,
		ownershipMonitor: newOwnershipMonitor,
		runApp:           app.Run,
	}
}

func runEngine(
	ctx context.Context,
	mode engineMode,
	home string,
	ownership io.Reader,
	stdout, stderr io.Writer,
) int {
	return runEngineWithDependencies(ctx, mode, home, ownership, stdout, stderr, defaultEngineDependencies())
}

func runEngineWithDependencies(
	ctx context.Context,
	mode engineMode,
	home string,
	ownership io.Reader,
	stdout, stderr io.Writer,
	dependencies engineDependencies,
) int {
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	signalCtx, stopSignals := notifySignals(ctx)
	defer stopSignals()

	// **所有権はロックより先である。** 持ち主が既に居なくなっていたなら、
	// ロックを取ってはならない——取れば、誰も待っていないエンジンが 1 台分の
	// 席を占める。
	var monitor ownershipMonitor
	var ownershipEvents <-chan error
	if mode == engineDesktop {
		created, err := dependencies.ownershipMonitor(ownership)
		if err != nil {
			fmt.Fprintln(stderr, "sshc: "+ownershipMessage(err))
			return 1
		}
		monitor = created
		events, err := monitor.Start(signalCtx)
		if err != nil {
			fmt.Fprintln(stderr, "sshc: "+ownershipMessage(err))
			return 1
		}
		ownershipEvents = events
	}

	release, err := dependencies.acquire(app.HandoffDir(home))
	if err != nil {
		if monitor != nil {
			_ = monitor.Stop()
		}
		switch {
		case errors.Is(err, errEngineRunning) && mode == engineDesktop:
			return engineBusyExit
		case errors.Is(err, errEngineRunning):
			fmt.Fprintln(stderr, "sshc: an sshc engine is already running; stop it before starting another")
			return 1
		}
		logger.Error("take the engine lock", "error", err)
		return 1
	}

	code := runEngineApp(signalCtx, mode, home, stdout, logger, ownershipEvents, dependencies)

	if monitor != nil {
		if err := monitor.Stop(); err != nil {
			logger.Error("stop the ownership monitor", "error", err)
			code = 1
		}
	}
	// **ロックを手放すのは最後である。** これより後に状態を変えるものは何も無い。
	if err := release(); err != nil {
		logger.Error("release the engine lock", "error", err)
		return 1
	}
	return code
}

// runEngineApp は、アプリケーションを走らせ、終わった理由を終了コードへ写す。
func runEngineApp(
	ctx context.Context,
	mode engineMode,
	home string,
	stdout io.Writer,
	logger *slog.Logger,
	ownershipEvents <-chan error,
	dependencies engineDependencies,
) int {
	assets, err := ui.FS()
	if err != nil {
		logger.Error("load embedded UI", "error", err)
		return 1
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go func() {
		select {
		case <-ctx.Done():
			cancel(context.Cause(ctx))
		case cause := <-ownershipEvents:
			if cause == nil {
				cause = errOwnershipEnded
			}
			cancel(cause)
		case <-runCtx.Done():
		}
	}()

	owner := handoff.OwnerHeadless
	if mode == engineDesktop {
		owner = handoff.OwnerDesktop
	}
	parts := newPlatformParts()
	dependencyValues := app.Dependencies{
		Random:   rand.Reader,
		Announce: announceReadiness(mode, stdout),
		// このアプリケーションが自分自身以外のホストに接触する唯一の場所であり、
		// 誰かが求めたときにだけ行う。何も取得せず、何も置き換えない。
		Updates: &selfupdate.Checker{
			API:  "https://api.github.com/repos/aida0710/sshc/releases/latest",
			HTTP: &http.Client{Timeout: 30 * time.Second},
		},
		Listen:          net.Listen,
		UI:              assets,
		Logger:          logger,
		Home:            home,
		Owner:           owner,
		PID:             os.Getpid(),
		Toolchain:       parts.Toolchain,
		KeyAgent:        parts.KeyAgent,
		Biometric:       parts.Biometric,
		Lookup:          os.LookupEnv,
		Environ:         os.Environ,
		ShutdownTimeout: dependencies.shutdownTimeout,
	}

	runErr := dependencies.runApp(runCtx, dependencyValues, version)
	cause := context.Cause(runCtx)
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		logger.Error("sshc stopped", "error", runErr)
		return 1
	}
	return exitForCause(cause, logger)
}

// exitForCause は、走行が終わった理由ひとつを終了コードへ写す。
func exitForCause(cause error, logger *slog.Logger) int {
	switch {
	case errors.Is(cause, errEngineInterrupted):
		return 130
	case errors.Is(cause, errOwnershipProtocol), errors.Is(cause, errOwnershipRead):
		// **秘密は書かない。** 理由の名前だけを残す。
		logger.Error("desktop ownership failed", "cause", cause.Error())
		return 1
	}
	// 正常終了、SIGTERM、呼び出し側の取り消し、そして所有権の EOF。
	return 0
}

// announceReadiness は、受付が始まったことをこの owner にふさわしい形で伝える。
func announceReadiness(mode engineMode, stdout io.Writer) func(app.Readiness) error {
	return func(readiness app.Readiness) error {
		if mode == engineDesktop {
			// 入口の URL は、Electron が読む私的な stdout だけへ出る。
			_, err := fmt.Fprintln(stdout, readiness.DesktopURL)
			return err
		}
		// **headless は入口を出さない。** 出せば、ログにも端末にもワンタイムの
		// 資格情報が残る。ここで言うのは、次に何を打てばよいかだけである。
		message := "sshc: create the password vault with `sshc vault create`"
		if readiness.VaultExists {
			message = "sshc: unlock the password vault with `sshc vault unlock`"
		}
		_, err := fmt.Fprintln(stdout, message)
		return err
	}
}

func ownershipMessage(err error) string {
	switch {
	case errors.Is(err, errOwnershipEnded):
		return "the desktop application closed the engine ownership channel before the engine started"
	case errors.Is(err, errOwnershipProtocol):
		return "the desktop engine was started without a usable ownership channel"
	default:
		return "the desktop engine could not watch its ownership channel"
	}
}

// watchSignals は、OS 別のシグナル集合をひとつの理由付き context に変える。
func watchSignals(ctx context.Context, signals chan os.Signal, stop func()) (context.Context, func()) {
	signalCtx, cancel := context.WithCancelCause(ctx)
	go func() {
		select {
		case received := <-signals:
			if received == os.Interrupt {
				cancel(errEngineInterrupted)
				return
			}
			cancel(errEngineTerminated)
		case <-signalCtx.Done():
		}
	}()
	return signalCtx, func() {
		stop()
		cancel(nil)
		signal.Stop(signals)
	}
}
