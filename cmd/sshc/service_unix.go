//go:build !windows

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"sshc/internal/app"
	"sshc/internal/enginelock"
	"sshc/internal/handoff"
)

// systemd（Linux）と launchd（macOS）の service 管理が共有する骨格。ツールの探索、
// コマンドの実行、操作の排他、engine の準備完了待ちは同じであり、違うのは
// ツール名と、その OS の定義ファイルとコマンド語彙だけである。

// serviceReadyPollInterval と serviceReadyProbeTimeout は、再起動した engine の
// handoff と status を確かめる周期と 1 回の HTTP 確認の上限である。
const (
	serviceReadyPollInterval = 50 * time.Millisecond
	serviceReadyProbeTimeout = 250 * time.Millisecond
)

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

// serviceOperationLock は、install／disable／restart を同じ home で直列化する。
func serviceOperationLock(home string) func() (func() error, error) {
	path := filepath.Join(home, ".config", "sshc", "service.mutation.lock")
	return func() (func() error, error) {
		release, err := enginelock.Acquire(path)
		if errors.Is(err, enginelock.ErrRunning) {
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

// engineReadiness は、OS ごとの「service の main PID は何か」と「失敗の説明」を
// 準備完了待ちへ渡す。
type engineReadiness struct {
	// mainPID は service の main process の PID を返す。まだ無ければ 0。
	mainPID func(context.Context) int
	// failureDetail は、間に合わなかったときにツールが報告する状態を返す。
	failureDetail func(context.Context) string
}

// waitForEngineReady は、service の main PID が handoff の PID と一致し、その engine が
// 自分の handoff どおりの status を返すまで待つ。手で起動した engine が同じ home に
// 居れば handoff は別の PID を指すので、そのことを案内して失敗する。
func waitForEngineReady(ctx context.Context, home string, readiness engineReadiness) error {
	readyCtx, cancel := context.WithTimeout(ctx, serviceReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(serviceReadyPollInterval)
	defer ticker.Stop()
	client := &http.Client{Timeout: serviceReadyProbeTimeout}

	for {
		if pid := readiness.mainPID(readyCtx); pid > 0 {
			document, readErr := handoff.Read(app.HandoffDir(home))
			if readErr == nil && document.PID == pid {
				if _, statusErr := requestStatus(readyCtx, document, client); statusErr == nil {
					return nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-readyCtx.Done():
			detail := readiness.failureDetail(ctx)
			if detail == "" {
				detail = "engine readiness was not published"
			}
			return fmt.Errorf("%s; stop any manually running `sshc engine` and retry", detail)
		case <-ticker.C:
		}
	}
}
