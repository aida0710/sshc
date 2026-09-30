//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"sshc/internal/storage"
)

const (
	serviceUnitName   = "sshc.service"
	serviceUnitMarker = "# Managed by sshc service install; schema=1\n"
)

var defaultSystemctlCandidates = []string{"/usr/bin/systemctl", "/bin/systemctl"}

type linuxServiceManager struct {
	home      string
	runner    serviceCommandRunner
	files     storage.FileSystem
	waitReady func(context.Context, string, serviceCommandRunner) error
	lock      func() (func() error, error)
}

func newPlatformServiceManager(home string) (engineServiceManager, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("home directory is not absolute")
	}
	manager := newServiceManagerWithoutTool(filepath.Clean(home))
	if err := manager.resolveTool(); err != nil {
		return nil, err
	}
	return manager, nil
}

// newServiceManagerWithoutTool は、systemctl をまだ探していない manager を返す。
// home は絶対パスで、Clean 済みであること。
func newServiceManagerWithoutTool(home string) *linuxServiceManager {
	return &linuxServiceManager{
		home:      home,
		files:     storage.OSFileSystem{},
		waitReady: waitForServiceReady,
		lock:      serviceOperationLock(home),
	}
}

// resolveTool は、systemctl を探して、この manager が実行するツールにする。
func (manager *linuxServiceManager) resolveTool() error {
	systemctl, err := resolveSystemctl(defaultSystemctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return err
	}
	manager.runner = osServiceCommandRunner{path: systemctl}
	return nil
}

func resolveSystemctl(
	candidates []string,
	lookPath func(string) (string, error),
	stat func(string) (os.FileInfo, error),
) (string, error) {
	return resolveServiceTool("systemctl", candidates, lookPath, stat)
}

func (manager *linuxServiceManager) unitPath() string {
	return filepath.Join(manager.home, ".config", "systemd", "user", serviceUnitName)
}

func (manager *linuxServiceManager) definitionFile() serviceDefinitionFile {
	return serviceDefinitionFile{
		files:  manager.files,
		path:   manager.unitPath(),
		marker: serviceUnitMarker,
		name:   serviceUnitName,
		render: systemdUnit,
	}
}

func (manager *linuxServiceManager) InstallPlan(executable string) (string, error) {
	if _, err := systemdUnit(executable); err != nil {
		return "", err
	}
	return fmt.Sprintf("install, enable, and start the systemd user service at %s using %s", manager.unitPath(), filepath.Clean(executable)), nil
}

func (manager *linuxServiceManager) DisablePlan() string {
	return fmt.Sprintf("stop, disable, and remove the systemd user service at %s", manager.unitPath())
}

func (manager *linuxServiceManager) Install(ctx context.Context, executable string) (result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, release()) }()
	definition := manager.definitionFile()
	snapshot, err := definition.readSnapshot()
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
	if err := manager.files.MkdirAll(filepath.Dir(manager.unitPath()), 0o700); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := definition.ensureUnchanged(snapshot); err != nil {
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
	return definition.ensureStillMatches(executable, "starting")
}

func (manager *linuxServiceManager) Status(ctx context.Context) (serviceState, error) {
	snapshot, err := manager.definitionFile().readSnapshot()
	if err != nil || snapshot.state == serviceAbsent || snapshot.state == serviceUnmanaged {
		return snapshot.state, err
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

func (manager *linuxServiceManager) RestartIfActive(ctx context.Context, executable string) (bool, error) {
	return restartServiceIfActive(ctx, manager, executable)
}

func (manager *linuxServiceManager) IsDefinitionOutdated() (bool, error) {
	return manager.definitionFile().isOutdated()
}

func (manager *linuxServiceManager) restartRunning(ctx context.Context) error {
	return manager.run(ctx, "--user", "try-restart", serviceUnitName)
}

func (manager *linuxServiceManager) acquireOperationLock() (func() error, error) {
	return acquireServiceOperationLock(manager.lock)
}

func (manager *linuxServiceManager) waitUntilReady(ctx context.Context) error {
	if manager.waitReady == nil {
		return errServiceReadinessUnavailable
	}
	return manager.waitReady(ctx, manager.home, manager.runner)
}

func waitForServiceReady(ctx context.Context, home string, runner serviceCommandRunner) error {
	return waitForEngineReady(ctx, home, engineReadiness{
		mainPID: func(ctx context.Context) int {
			result, err := runner.Run(ctx, "--user", "show", "--property=MainPID", "--value", serviceUnitName)
			if err != nil || result.ExitCode != 0 {
				return 0
			}
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(result.Output)))
			if parseErr != nil {
				return 0
			}
			return pid
		},
		failureDetail: func(ctx context.Context) string { return serviceFailureDetail(ctx, runner) },
	})
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

func (manager *linuxServiceManager) Disable(ctx context.Context) (removed bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	definition := manager.definitionFile()
	snapshot, err := definition.readSnapshot()
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
	if err := definition.ensureUnchanged(snapshot); err != nil {
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
	return serviceToolExitError("systemctl", arguments, result)
}

func systemdUnit(executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("service executable path is not absolute")
	}
	if containsControl(executable) {
		return "", errors.New("service executable path contains a control character")
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
