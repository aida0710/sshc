package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"sshc/internal/app"
	"sshc/internal/filelock"
)

// systemd、launchd、Windowsのタスクが共有するサービス管理。ツールの探索、
// コマンドの実行、操作の排他、engine の準備完了待ちは同じであり、違うのは
// ツール名と、その OS の定義ファイルとコマンド語彙だけである。

type serviceCommandResult struct {
	ExitCode int
	Output   []byte
}

type serviceCommandRunner interface {
	Run(context.Context, ...string) (serviceCommandResult, error)
}

// osServiceCommandRunner は、探索済みの絶対 path のツールを起動する。
type osServiceCommandRunner struct {
	path string
}

func (runner osServiceCommandRunner) Run(ctx context.Context, arguments ...string) (serviceCommandResult, error) {
	command := exec.CommandContext(ctx, runner.path, arguments...)
	output, err := command.CombinedOutput()
	if err == nil {
		return serviceCommandResult{ExitCode: 0, Output: output}, nil
	}
	if ctx.Err() != nil {
		return serviceCommandResult{}, ctx.Err()
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return serviceCommandResult{ExitCode: exitError.ExitCode(), Output: output}, nil
	}
	return serviceCommandResult{}, err
}

// errServiceReadinessUnavailable は、manager に engine の準備完了の確かめ方が渡されて
// いないことを表す。
var errServiceReadinessUnavailable = errors.New("service readiness check is unavailable")

// acquireServiceOperationLock は、manager に排他の手段が渡されていなければ操作を始めない。
func acquireServiceOperationLock(lock func() (func() error, error)) (func() error, error) {
	if lock == nil {
		return nil, errors.New("service operation lock is unavailable")
	}
	return lock()
}

// serviceRestartDefinition keeps file and scheduler definitions behind the same checks.
type serviceRestartDefinition interface {
	matches(string) (bool, error)
	ensureStillMatches(string, string) error
}

type restartableService interface {
	restartDefinition(context.Context) serviceRestartDefinition
	acquireOperationLock() (func() error, error)
	Status(context.Context) (serviceState, error)
	// restartRunning は、動いている service だけを再起動する（systemctl try-restart、
	// launchctl kickstart -k、Windowsでは正常終了を待ってschtasks /Run）。
	restartRunning(context.Context) error
	waitUntilReady(context.Context) error
}

// restartServiceIfActive は、sshc 管理下で今動いている service だけを再起動する。停止中の
// service を update や明示restartで勝手に起動せず、手書きの定義にも触れない境界である。
func restartServiceIfActive(ctx context.Context, service restartableService, executable string) (restarted bool, result error) {
	release, err := service.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	definition := service.restartDefinition(ctx)
	matches, err := definition.matches(executable)
	if err != nil || !matches {
		return false, err
	}
	state, err := service.Status(ctx)
	if err != nil || state != serviceActive {
		return false, err
	}
	// 状態を確かめる間に定義が差し替わっていれば、その service は再起動しない。
	matches, err = definition.matches(executable)
	if err != nil || !matches {
		return false, err
	}
	if err := service.restartRunning(ctx); err != nil {
		return false, err
	}
	// try-restart と kickstart は、その間に service が止まっていても成功する。止まった
	// service を起こしたことにせず、動いていることを確かめてから準備完了を待つ。
	state, err = service.Status(ctx)
	if err != nil || state != serviceActive {
		return false, err
	}
	if err := service.waitUntilReady(ctx); err != nil {
		return false, fmt.Errorf("restarted service did not become ready: %w", err)
	}
	if err := definition.ensureStillMatches(executable, "restarting"); err != nil {
		return false, err
	}
	return true, nil
}

// failedService は、起動の失敗が続いて OS が起動し直すのをやめた service を起動し直せる manager である。
type failedService interface {
	restartDefinition(context.Context) serviceRestartDefinition
	acquireOperationLock() (func() error, error)
	// stoppedByFailure は、service が止まっている理由が起動の失敗かを返す。利用者が sshc stop で
	// 止めた service（正常に終わった service）は false である。
	stoppedByFailure(context.Context) (bool, error)
	// startAfterFailure は、OS が数えた失敗を消してから service を起動する。
	startAfterFailure(context.Context) error
	waitUntilReady(context.Context) error
}

// startServiceIfFailed は、起動の失敗で止まった service を、入れたばかりの実行ファイルで起動し直す。
// 起動のたびに落ちる版から直した版へ上げたとき、service が止まったまま残らないようにする。利用者が
// 止めた service は起動しない（restartServiceIfActive と同じ境界）。
func startServiceIfFailed(ctx context.Context, service failedService, executable string) (started bool, result error) {
	release, err := service.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	definition := service.restartDefinition(ctx)
	matches, err := definition.matches(executable)
	if err != nil || !matches {
		return false, err
	}
	failed, err := service.stoppedByFailure(ctx)
	if err != nil || !failed {
		return false, err
	}
	if err := service.startAfterFailure(ctx); err != nil {
		return false, err
	}
	if err := service.waitUntilReady(ctx); err != nil {
		return false, fmt.Errorf("started service did not become ready: %w", err)
	}
	if err := definition.ensureStillMatches(executable, "starting"); err != nil {
		return false, err
	}
	return true, nil
}

// serviceOperationLock は、install／disable／restart を同じ home で直列化する。
func serviceOperationLock(home string) func() (func() error, error) {
	path := filepath.Join(home, ".config", "sshc", "service.mutation.lock")
	if runtime.GOOS == "windows" {
		stateDir, err := app.StateDir(home)
		if err != nil {
			return func() (func() error, error) { return nil, err }
		}
		path = filepath.Join(stateDir, "service.mutation.lock")
	}
	return func() (func() error, error) {
		release, err := filelock.TryAcquire(path)
		if errors.Is(err, filelock.ErrHeld) {
			return nil, errors.New("another sshc service operation is in progress")
		}
		return release, err
	}
}

// resolveServiceTool は、固定の候補と PATH 上の tool から、実行できる絶対 path を
// ひとつ選ぶ。制御文字を含む path はどの OS でも使わない。
func resolveServiceTool(
	tool string,
	candidates []string,
	lookPath func(string) (string, error),
	stat func(string) (os.FileInfo, error),
) (string, error) {
	paths := append([]string(nil), candidates...)
	if found, err := lookPath(tool); err == nil {
		if !filepath.IsAbs(found) {
			found, err = filepath.Abs(found)
			if err != nil {
				return "", fmt.Errorf("resolve %s path: %w", tool, err)
			}
		}
		paths = append(paths, filepath.Clean(found))
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) || containsControl(path) {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("cannot find an executable %s", tool)
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

// serviceToolExitError は、失敗したコマンドを出力付きで説明する。
func serviceToolExitError(tool string, arguments []string, result serviceCommandResult) error {
	detail := strings.TrimSpace(string(result.Output))
	if detail == "" {
		return fmt.Errorf("%s %s exited with status %d", tool, strings.Join(arguments, " "), result.ExitCode)
	}
	return fmt.Errorf("%s %s exited with status %d: %s", tool, strings.Join(arguments, " "), result.ExitCode, detail)
}
