//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"sshc/internal/storage"
)

const (
	serviceUnitName   = "sshc.service"
	serviceUnitMarker = "# Managed by sshc service install; schema=1\n"
	defaultSystemctl  = "/usr/bin/systemctl"
)

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
	home   string
	runner serviceCommandRunner
	files  storage.FileSystem
}

func newPlatformServiceManager(home string) (engineServiceManager, error) {
	if !filepath.IsAbs(home) {
		return nil, errors.New("home directory is not absolute")
	}
	info, err := os.Stat(defaultSystemctl)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", defaultSystemctl, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("%s is not executable", defaultSystemctl)
	}
	return &linuxServiceManager{
		home:   filepath.Clean(home),
		runner: osServiceCommandRunner{path: defaultSystemctl},
		files:  storage.OSFileSystem{},
	}, nil
}

func (manager *linuxServiceManager) unitPath() string {
	return filepath.Join(manager.home, ".config", "systemd", "user", serviceUnitName)
}

func (manager *linuxServiceManager) Install(ctx context.Context, executable string) error {
	state, err := manager.unitState()
	if err != nil {
		return err
	}
	if state == serviceUnmanaged {
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
	return manager.run(ctx, "--user", "restart", serviceUnitName)
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

func (manager *linuxServiceManager) Disable(ctx context.Context) (bool, error) {
	state, err := manager.unitState()
	if err != nil {
		return false, err
	}
	if state == serviceAbsent {
		return false, nil
	}
	if state == serviceUnmanaged {
		return false, errUnmanagedServiceUnit
	}
	if err := manager.run(ctx, "--user", "disable", "--now", serviceUnitName); err != nil {
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
	contents, err := manager.files.ReadFile(manager.unitPath())
	if errors.Is(err, os.ErrNotExist) {
		return serviceAbsent, nil
	}
	if err != nil {
		return serviceAbsent, fmt.Errorf("read %s: %w", manager.unitPath(), err)
	}
	if !strings.HasPrefix(string(contents), serviceUnitMarker) {
		return serviceUnmanaged, nil
	}
	return serviceInactive, nil
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
