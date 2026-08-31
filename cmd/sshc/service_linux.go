//go:build linux

package main

import (
	"bytes"
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
)

var defaultSystemctlCandidates = []string{"/usr/bin/systemctl", "/bin/systemctl"}

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
	systemctl, err := resolveSystemctl(defaultSystemctlCandidates, exec.LookPath, os.Stat)
	if err != nil {
		return nil, err
	}
	return &linuxServiceManager{
		home:   filepath.Clean(home),
		runner: osServiceCommandRunner{path: systemctl},
		files:  storage.OSFileSystem{},
	}, nil
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

// RestartIfActive はsshc管理下で現在動作中のunitだけを再起動する。停止中のunitを
// updateが勝手に起動せず、手書きunitにも触れないための更新連携用境界である。
func (manager *linuxServiceManager) RestartIfActive(ctx context.Context, executable string) (bool, error) {
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
	if err := manager.run(ctx, "--user", "try-restart", serviceUnitName); err != nil {
		return false, err
	}
	return true, nil
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
