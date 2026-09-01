//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/storage"
)

const (
	launchdServiceLabel = "io.github.aida0710.sshc"
	launchdPlistName    = launchdServiceLabel + ".plist"
	launchdPlistMarker  = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- Managed by sshc service install; schema=1 -->\n"
)

var launchdPIDPattern = regexp.MustCompile(`(?m)^\s*pid\s*=\s*([0-9]+)\s*$`)

type launchdCommandResult struct {
	ExitCode int
	Output   []byte
}

type launchdCommandRunner interface {
	Run(context.Context, ...string) (launchdCommandResult, error)
}

type osLaunchdCommandRunner struct{ path string }

func (runner osLaunchdCommandRunner) Run(ctx context.Context, arguments ...string) (launchdCommandResult, error) {
	command := exec.CommandContext(ctx, runner.path, arguments...)
	output, err := command.CombinedOutput()
	if err == nil {
		return launchdCommandResult{Output: output}, nil
	}
	if ctx.Err() != nil {
		return launchdCommandResult{}, ctx.Err()
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return launchdCommandResult{ExitCode: exitError.ExitCode(), Output: output}, nil
	}
	return launchdCommandResult{}, err
}

type launchdServiceManager struct {
	home      string
	uid       int
	runner    launchdCommandRunner
	files     storage.FileSystem
	waitReady func(context.Context, string, int, launchdCommandRunner) error
	lock      func() (func() error, error)
}

type launchdPlistSnapshot struct {
	state    serviceState
	contents []byte
}

func (manager *launchdServiceManager) plistPath() string {
	return filepath.Join(manager.home, "Library", "LaunchAgents", launchdPlistName)
}

func (manager *launchdServiceManager) domain() string {
	return fmt.Sprintf("gui/%d", manager.uid)
}

func (manager *launchdServiceManager) target() string {
	return manager.domain() + "/" + launchdServiceLabel
}

func (manager *launchdServiceManager) InstallPlan(executable string) (string, error) {
	if _, err := launchdPlist(executable, manager.home); err != nil {
		return "", err
	}
	return fmt.Sprintf("install and start the launchd user agent at %s using %s", manager.plistPath(), filepath.Clean(executable)), nil
}

func (manager *launchdServiceManager) DisablePlan() string {
	return fmt.Sprintf("stop and remove the launchd user agent at %s", manager.plistPath())
}

func (manager *launchdServiceManager) Install(ctx context.Context, executable string) (result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readPlistSnapshot()
	if err != nil {
		return err
	}
	if snapshot.state == serviceUnmanaged {
		return errUnmanagedServiceUnit
	}
	plist, err := launchdPlist(executable, manager.home)
	if err != nil {
		return err
	}
	loaded, err := manager.isLoaded(ctx)
	if err != nil {
		return err
	}
	if loaded {
		if err := manager.run(ctx, "bootout", manager.target()); err != nil {
			return err
		}
	}
	if err := manager.ensurePlistUnchanged(snapshot); err != nil {
		return err
	}
	if err := manager.files.MkdirAll(filepath.Dir(manager.plistPath()), 0o700); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := storage.WriteAtomicFile(manager.files, manager.plistPath(), ".sshc-launchd-", 0o600, []byte(plist)); err != nil {
		return fmt.Errorf("write %s: %w", manager.plistPath(), err)
	}
	if err := manager.run(ctx, "bootstrap", manager.domain(), manager.plistPath()); err != nil {
		return err
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return fmt.Errorf("service did not become ready: %w", err)
	}
	matches, err := manager.plistMatches(executable)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("launchd service definition changed while the service was starting")
	}
	return nil
}

func (manager *launchdServiceManager) Status(ctx context.Context) (serviceState, error) {
	snapshot, err := manager.readPlistSnapshot()
	if err != nil || snapshot.state == serviceAbsent || snapshot.state == serviceUnmanaged {
		return snapshot.state, err
	}
	loaded, pid, err := manager.inspectJob(ctx)
	if err != nil {
		return serviceInactive, err
	}
	if loaded && pid > 0 {
		return serviceActive, nil
	}
	return serviceInactive, nil
}

func (manager *launchdServiceManager) RestartIfActive(ctx context.Context, executable string) (restarted bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	matches, err := manager.plistMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	state, err := manager.Status(ctx)
	if err != nil || state != serviceActive {
		return false, err
	}
	matches, err = manager.plistMatches(executable)
	if err != nil || !matches {
		return false, err
	}
	if err := manager.run(ctx, "kickstart", "-k", manager.target()); err != nil {
		return false, err
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return false, fmt.Errorf("restarted service did not become ready: %w", err)
	}
	matches, err = manager.plistMatches(executable)
	if err != nil {
		return false, err
	}
	if !matches {
		return false, errors.New("launchd service definition changed while the service was restarting")
	}
	return true, nil
}

func (manager *launchdServiceManager) Disable(ctx context.Context) (removed bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readPlistSnapshot()
	if err != nil {
		return false, err
	}
	if snapshot.state == serviceAbsent {
		return false, nil
	}
	if snapshot.state == serviceUnmanaged {
		return false, errUnmanagedServiceUnit
	}
	loaded, err := manager.isLoaded(ctx)
	if err != nil {
		return false, err
	}
	if loaded {
		if err := manager.run(ctx, "bootout", manager.target()); err != nil {
			return false, err
		}
	}
	if err := manager.ensurePlistUnchanged(snapshot); err != nil {
		return false, err
	}
	if err := manager.files.Remove(manager.plistPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove %s: %w", manager.plistPath(), err)
	}
	return true, nil
}

func (manager *launchdServiceManager) acquireOperationLock() (func() error, error) {
	if manager.lock == nil {
		return nil, errors.New("service operation lock is unavailable")
	}
	return manager.lock()
}

func (manager *launchdServiceManager) waitUntilReady(ctx context.Context) error {
	if manager.waitReady == nil {
		return errors.New("service readiness check is unavailable")
	}
	return manager.waitReady(ctx, manager.home, manager.uid, manager.runner)
}

func (manager *launchdServiceManager) readPlistSnapshot() (launchdPlistSnapshot, error) {
	contents, err := manager.files.ReadFile(manager.plistPath())
	if errors.Is(err, os.ErrNotExist) {
		return launchdPlistSnapshot{state: serviceAbsent}, nil
	}
	if err != nil {
		return launchdPlistSnapshot{}, fmt.Errorf("read %s: %w", manager.plistPath(), err)
	}
	if !bytes.HasPrefix(contents, []byte(launchdPlistMarker)) {
		return launchdPlistSnapshot{state: serviceUnmanaged, contents: contents}, nil
	}
	return launchdPlistSnapshot{state: serviceInactive, contents: contents}, nil
}

func (manager *launchdServiceManager) ensurePlistUnchanged(expected launchdPlistSnapshot) error {
	actual, err := manager.readPlistSnapshot()
	if err != nil {
		return err
	}
	if actual.state != expected.state || !bytes.Equal(actual.contents, expected.contents) {
		return errors.New("launchd service definition changed during the operation; it was left in place")
	}
	return nil
}

func (manager *launchdServiceManager) plistMatches(executable string) (bool, error) {
	expected, err := launchdPlist(executable, manager.home)
	if err != nil {
		return false, err
	}
	contents, err := manager.files.ReadFile(manager.plistPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", manager.plistPath(), err)
	}
	return bytes.Equal(contents, []byte(expected)), nil
}

func (manager *launchdServiceManager) isLoaded(ctx context.Context) (bool, error) {
	loaded, _, err := manager.inspectJob(ctx)
	return loaded, err
}

func (manager *launchdServiceManager) inspectJob(ctx context.Context) (bool, int, error) {
	result, err := manager.runner.Run(ctx, "print", manager.target())
	if err != nil {
		return false, 0, err
	}
	if result.ExitCode == 0 {
		pid := 0
		if match := launchdPIDPattern.FindSubmatch(result.Output); len(match) == 2 {
			pid, _ = strconv.Atoi(string(match[1]))
		}
		return true, pid, nil
	}
	if launchdServiceNotFound(result) {
		return false, 0, nil
	}
	return false, 0, launchctlExitError([]string{"print", manager.target()}, result)
}

func launchdServiceNotFound(result launchdCommandResult) bool {
	detail := strings.ToLower(string(result.Output))
	return result.ExitCode == 113 || strings.Contains(detail, "could not find service") || strings.Contains(detail, "service not found")
}

func (manager *launchdServiceManager) run(ctx context.Context, arguments ...string) error {
	result, err := manager.runner.Run(ctx, arguments...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return launchctlExitError(arguments, result)
	}
	return nil
}

func launchctlExitError(arguments []string, result launchdCommandResult) error {
	detail := strings.TrimSpace(string(result.Output))
	if detail == "" {
		return fmt.Errorf("launchctl %s exited with status %d", strings.Join(arguments, " "), result.ExitCode)
	}
	return fmt.Errorf("launchctl %s exited with status %d: %s", strings.Join(arguments, " "), result.ExitCode, detail)
}

func launchdPlist(executable, home string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("service executable path is not absolute")
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("home directory is not absolute")
	}
	if containsControl(executable) || containsControl(home) {
		return "", errors.New("service path contains a control character")
	}
	executable = filepath.Clean(executable)
	home = filepath.Clean(home)
	return launchdPlistMarker +
		"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
		"<plist version=\"1.0\">\n<dict>\n" +
		"  <key>Label</key>\n  <string>" + xmlText(launchdServiceLabel) + "</string>\n" +
		"  <key>ProgramArguments</key>\n  <array>\n    <string>" + xmlText(executable) + "</string>\n    <string>engine</string>\n  </array>\n" +
		"  <key>WorkingDirectory</key>\n  <string>" + xmlText(home) + "</string>\n" +
		"  <key>RunAtLoad</key>\n  <true/>\n" +
		"  <key>KeepAlive</key>\n  <true/>\n" +
		"  <key>ThrottleInterval</key>\n  <integer>5</integer>\n" +
		"</dict>\n</plist>\n", nil
}

func xmlText(value string) string {
	var output bytes.Buffer
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func waitForLaunchdServiceReady(ctx context.Context, home string, uid int, runner launchdCommandRunner) error {
	readyCtx, cancel := context.WithTimeout(ctx, serviceReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	target := fmt.Sprintf("gui/%d/%s", uid, launchdServiceLabel)

	for {
		result, err := runner.Run(readyCtx, "print", target)
		if err == nil && result.ExitCode == 0 {
			match := launchdPIDPattern.FindSubmatch(result.Output)
			if len(match) == 2 {
				pid, parseErr := strconv.Atoi(string(match[1]))
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
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-readyCtx.Done():
			detail := launchdFailureDetail(ctx, runner, target)
			if detail == "" {
				detail = "engine readiness was not published"
			}
			return fmt.Errorf("%s; stop any manually running `sshc engine` and retry", detail)
		case <-ticker.C:
		}
	}
}

func launchdFailureDetail(ctx context.Context, runner launchdCommandRunner, target string) string {
	result, err := runner.Run(ctx, "print", target)
	if err != nil || result.ExitCode != 0 {
		return ""
	}
	for _, line := range strings.Split(string(result.Output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "state =") || strings.HasPrefix(line, "last exit code =") {
			return "launchd reports " + line
		}
	}
	return ""
}
