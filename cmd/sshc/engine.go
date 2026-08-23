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
	acquire         func(string) (func() error, error)
	runApp          func(context.Context, app.Dependencies, string) error
	shutdownTimeout time.Duration
}

func defaultEngineDependencies() engineDependencies {
	return engineDependencies{
		acquire: lockEngineStart,
		runApp:  app.Run,
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
	home string,
	options engineOptions,
	stdin io.Reader,
	stdout, stderr io.Writer,
) int {
	return runEngineWithDependencies(ctx, home, options, stdin, stdout, stderr, defaultEngineDependencies())
}

func runEngineWithDependencies(
	ctx context.Context,
	home string,
	options engineOptions,
	stdin io.Reader,
	stdout, stderr io.Writer,
	dependencies engineDependencies,
) int {
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	signalCtx, stopSignals := notifySignals(ctx)
	defer stopSignals()

	// **所有権はロックより先である。** 持ち主が既に居なくなっていたなら、
	// ロックを取ってはならない——取れば、誰も待っていないエンジンが 1 台分の
	// 席を占める。
	release, err := dependencies.acquire(app.HandoffDir(home))
	if errors.Is(err, errEngineRunning) {
		// **2 台目は立てない。** ただし、走っているものを止める道は要る
		// ——どこで起こしたか分からない engine を、探して回らずに畳めるように。
		taken, takeErr := replaceRunningEngine(ctx, home, options, stdin, stdout, stderr, dependencies.acquire)
		if takeErr != nil {
			// **断ったことは、もう綴ってある。** ここで重ねると同じ話が二度出る。
			if !errors.Is(takeErr, errAlreadyRunning) {
				fmt.Fprintf(stderr, "sshc: %v\n", takeErr)
			}
			return 1
		}
		release, err = taken, nil
	}
	if err != nil {
		logger.Error("take the engine lock", "error", err)
		return 1
	}

	code := runEngineApp(signalCtx, home, options, stdout, logger, dependencies)

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
	home string,
	options engineOptions,
	stdout io.Writer,
	logger *slog.Logger,
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
		case <-runCtx.Done():
		}
	}()

	parts := newPlatformParts()
	dependencyValues := app.Dependencies{
		Random:   rand.Reader,
		Port:     options.Port,
		Announce: announceReadiness(stdout),
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
		return 1
	}
	return exitForCause(cause, logger)
}

// exitForCause は、走行が終わった理由ひとつを終了コードへ写す。
func exitForCause(cause error, logger *slog.Logger) int {
	switch {
	case errors.Is(cause, errEngineInterrupted):
		return 130
	}
	// 正常終了、SIGTERM、呼び出し側の取り消し。
	return 0
}

// announceReadiness は、受付が始まったことを伝える。
//
// **入口はここに出さない。** 出せば、ログにも端末にもワンタイムの資格情報が
// 残る。入口は `sshc` が求めたときに 1 つずつ発行される。
func announceReadiness(stdout io.Writer) func(app.Readiness) error {
	return func(readiness app.Readiness) error {
		message := "sshc: create the password vault with `sshc vault create`"
		if readiness.VaultExists {
			message = "sshc: unlock the password vault with `sshc vault unlock`"
		}
		_, err := fmt.Fprintln(stdout, message)
		return err
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
