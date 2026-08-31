//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"sshc/internal/app"
	"sshc/internal/enginelock"
	"sshc/internal/handoff"
	"sshc/internal/storage"
)

const (
	serviceUnitName   = "sshc.service"
	serviceUnitMarker = "# Managed by sshc service install; schema=1\n"
)

var defaultSystemctlCandidates = []string{"/usr/bin/systemctl", "/bin/systemctl"}

const serviceReadyTimeout = 5 * time.Second

type serviceCommandResult struct {
	ExitCode int
	Output   []byte
}

type serviceCommandRunner interface {
	Run(context.Context, ...string) (serviceCommandResult, error)
}

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

type linuxServiceManager struct {
	home      string
	runner    serviceCommandRunner
	files     storage.FileSystem
	waitReady func(context.Context, string, serviceCommandRunner) error
	lock      func() (func() error, error)
}

type serviceUnitSnapshot struct {
	state    serviceState
	contents []byte
}

func newPlatformServiceManager(home string) (engineServiceManager, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("home directory is not absolute")
	}
	systemctl, err := resolveSystemctl(defaultSystemctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return nil, err
	}
	return &linuxServiceManager{
		home:      filepath.Clean(home),
		runner:    osServiceCommandRunner{path: systemctl},
		files:     storage.OSFileSystem{},
		waitReady: waitForServiceReady,
		lock:      serviceOperationLock(filepath.Clean(home)),
	}, nil
}

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

func resolveSystemctl(
	candidates []string,
	lookPath func(string) (string, error),
	stat func(string) (os.FileInfo, error),
) (string, error) {
	paths := append([]string(nil), candidates...)
	if found, err := lookPath("systemctl"); err == nil {
		if !filepath.IsAbs(found) {
			found, err = filepath.Abs(found)
			if err != nil {
				return "", fmt.Errorf("resolve systemctl path: %w", err)
			}
		}
		paths = append(paths, filepath.Clean(found))
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n\x00") {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		info, err := stat(path)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("cannot find an executable systemctl")
}

func (manager *linuxServiceManager) unitPath() string {
	return filepath.Join(manager.home, ".config", "systemd", "user", serviceUnitName)
}

func (manager *linuxServiceManager) Install(ctx context.Context, executable string) (result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readUnitSnapshot()
	if err != nil {
		return err
	}
	if snapshot.state == serviceUnmanaged {
		return errUnmanagedServiceUnit
	}
	unit, err := systemdUnit(executable)
	if err != nil {
		return err
	}
	directory := filepath.Dir(manager.unitPath())
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := manager.ensureUnitUnchanged(snapshot); err != nil {
		return err
	}
	if err := storage.WriteAtomicFile(manager.files, manager.unitPath(), ".sshc-service-", 0o600, []byte(unit)); err != nil {
		return fmt.Errorf("write %s: %w", manager.unitPath(), err)
	}
	if err := manager.run(ctx, "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := manager.run(ctx, "--user", "enable", serviceUnitName); err != nil {
		return err
	}
	// enable --nowは既に動いているunitのExecStartを更新しないため、再導入でも
	// 新しいbinaryへ確実に切り替わるよう明示的にrestartする。
	if err := manager.run(ctx, "--user", "restart", serviceUnitName); err != nil {
		return err
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return fmt.Errorf("service did not become ready: %w", err)
	}
	matches, err := manager.unitMatches(executable)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("sshc.service changed while the service was starting")
	}
	return nil
}

func (manager *linuxServiceManager) Status(ctx context.Context) (serviceState, error) {
	state, err := manager.unitState()
	if err != nil || state == serviceAbsent || state == serviceUnmanaged {
		return state, err
	}
	result, err := manager.runner.Run(ctx, "--user", "is-active", "--quiet", serviceUnitName)
	if err != nil {
		return serviceInactive, err
	}
	switch result.ExitCode {
	case 0:
		return serviceActive, nil
	case 3, 4:
		return serviceInactive, nil
	default:
		return serviceInactive, systemctlExitError([]string{"--user", "is-active", "--quiet", serviceUnitName}, result)
	}
}

// RestartIfActive はsshc管理下で現在動作中のunitだけを再起動する。停止中のunitを
// updateが勝手に起動せず、手書きunitにも触れないための更新連携用境界である。
func (manager *linuxServiceManager) RestartIfActive(ctx context.Context, executable string) (restarted bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	matches, err := manager.unitMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	state, err := manager.Status(ctx)
	if err != nil {
		return false, err
	}
	if state != serviceActive {
		return false, nil
	}
	matches, err = manager.unitMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	if err := manager.run(ctx, "--user", "try-restart", serviceUnitName); err != nil {
		return false, err
	}
	state, err = manager.Status(ctx)
	if err != nil {
		return false, err
	}
	if state != serviceActive {
		return false, nil
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return false, fmt.Errorf("restarted service did not become ready: %w", err)
	}
	matches, err = manager.unitMatches(executable)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, errors.New("sshc.service changed while the service was restarting")
	}
	return true, nil
}

func (manager *linuxServiceManager) acquireOperationLock() (func() error, error) {
	if manager.lock == nil {
		return nil, errors.New("service operation lock is unavailable")
	}
	return manager.lock()
}

func (manager *linuxServiceManager) waitUntilReady(ctx context.Context) error {
	if manager.waitReady == nil {
		return errors.New("service readiness check is unavailable")
	}
	return manager.waitReady(ctx, manager.home, manager.runner)
}

func waitForServiceReady(ctx context.Context, home string, runner serviceCommandRunner) error {
	readyCtx, cancel := context.WithTimeout(ctx, serviceReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 250 * time.Millisecond}

	for {
		result, err := runner.Run(readyCtx, "--user", "show", "--property=MainPID", "--value", serviceUnitName)
		if err == nil && result.ExitCode == 0 {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(result.Output)))
			if parseErr == nil && pid > 0 {
				document, readErr := handoff.Read(app.HandoffDir(home))
				if readErr == nil && document.PID == pid {
					answer, statusErr := requestStatus(readyCtx, document, client)
					if statusErr == nil && answer.Owner == document.Owner && answer.Version == document.Version &&
						answer.ProtocolVersion == document.ProtocolVersion {
						return nil
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-readyCtx.Done():
			detail := serviceFailureDetail(ctx, runner)
			if detail == "" {
				detail = "engine readiness was not published"
			}
			return fmt.Errorf("%s; stop any manually running `sshc engine` and retry", detail)
		case <-ticker.C:
		}
	}
}

func serviceFailureDetail(ctx context.Context, runner serviceCommandRunner) string {
	result, err := runner.Run(ctx, "--user", "show",
		"--property=ActiveState", "--property=SubState", "--property=Result", "--property=ExecMainStatus",
		"--value", serviceUnitName)
	if err != nil || result.ExitCode != 0 {
		return ""
	}
	lines := strings.Fields(string(result.Output))
	if len(lines) == 0 {
		return ""
	}
	return "systemd reports " + strings.Join(lines, "/")
}

func (manager *linuxServiceManager) unitMatches(executable string) (bool, error) {
	expected, err := systemdUnit(executable)
	if err != nil {
		return false, err
	}
	contents, err := manager.files.ReadFile(manager.unitPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", manager.unitPath(), err)
	}
	return bytes.Equal(contents, []byte(expected)), nil
}

func (manager *linuxServiceManager) Disable(ctx context.Context) (removed bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readUnitSnapshot()
	if err != nil {
		return false, err
	}
	if snapshot.state == serviceAbsent {
		return false, nil
	}
	if snapshot.state == serviceUnmanaged {
		return false, errUnmanagedServiceUnit
	}
	if err := manager.run(ctx, "--user", "disable", "--now", serviceUnitName); err != nil {
		return false, err
	}
	if err := manager.ensureUnitUnchanged(snapshot); err != nil {
		return false, err
	}
	if err := manager.files.Remove(manager.unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove %s: %w", manager.unitPath(), err)
	}
	if err := manager.run(ctx, "--user", "daemon-reload"); err != nil {
		return false, err
	}
	return true, nil
}

func (manager *linuxServiceManager) unitState() (serviceState, error) {
	snapshot, err := manager.readUnitSnapshot()
	return snapshot.state, err
}

func (manager *linuxServiceManager) readUnitSnapshot() (serviceUnitSnapshot, error) {
	contents, err := manager.files.ReadFile(manager.unitPath())
	if errors.Is(err, os.ErrNotExist) {
		return serviceUnitSnapshot{state: serviceAbsent}, nil
	}
	if err != nil {
		return serviceUnitSnapshot{state: serviceAbsent}, fmt.Errorf("read %s: %w", manager.unitPath(), err)
	}
	if !strings.HasPrefix(string(contents), serviceUnitMarker) {
		return serviceUnitSnapshot{state: serviceUnmanaged, contents: contents}, nil
	}
	return serviceUnitSnapshot{state: serviceInactive, contents: contents}, nil
}

func (manager *linuxServiceManager) ensureUnitUnchanged(expected serviceUnitSnapshot) error {
	actual, err := manager.readUnitSnapshot()
	if err != nil {
		return err
	}
	if actual.state != expected.state || !bytes.Equal(actual.contents, expected.contents) {
		return errors.New("sshc.service changed during the operation; it was left in place")
	}
	return nil
}

func (manager *linuxServiceManager) run(ctx context.Context, arguments ...string) error {
	result, err := manager.runner.Run(ctx, arguments...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return systemctlExitError(arguments, result)
	}
	return nil
}

func systemctlExitError(arguments []string, result serviceCommandResult) error {
	detail := strings.TrimSpace(string(result.Output))
	if detail == "" {
		return fmt.Errorf("systemctl %s exited with status %d", strings.Join(arguments, " "), result.ExitCode)
	}
	return fmt.Errorf("systemctl %s exited with status %d: %s", strings.Join(arguments, " "), result.ExitCode, detail)
}

func systemdUnit(executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("service executable path is not absolute")
	}
	for _, character := range executable {
		if unicode.IsControl(character) {
			return "", errors.New("service executable path contains a control character")
		}
	}
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		`%`, `%%`,
		`$`, `$$`,
	).Replace(filepath.Clean(executable))
	return serviceUnitMarker +
		"[Unit]\n" +
		"Description=sshc engine\n" +
		"After=default.target\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=\"" + escaped + "\" engine\n" +
		"Restart=on-failure\n" +
		"SuccessExitStatus=130\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n", nil
}
