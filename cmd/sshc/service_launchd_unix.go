//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sshc/internal/storage"
)

const (
	launchdServiceLabel = "io.github.aida0710.sshc"
	launchdPlistName    = launchdServiceLabel + ".plist"
	launchdPlistMarker  = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- Managed by sshc service install; schema=1 -->\n"
)

var launchdPIDPattern = regexp.MustCompile(`(?m)^\s*pid\s*=\s*([0-9]+)\s*$`)

type launchdServiceManager struct {
	home      string
	uid       int
	runner    serviceCommandRunner
	files     storage.FileSystem
	waitReady func(context.Context, *launchdServiceManager) error
	lock      func() (func() error, error)
}

func (manager *launchdServiceManager) plistPath() string {
	return filepath.Join(manager.home, "Library", "LaunchAgents", launchdPlistName)
}

func (manager *launchdServiceManager) definitionFile() serviceDefinitionFile {
	return serviceDefinitionFile{
		files:  manager.files,
		path:   manager.plistPath(),
		marker: launchdPlistMarker,
		name:   "launchd service definition",
		render: func(executable string) (string, error) { return launchdPlist(executable, manager.home) },
	}
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
	definition := manager.definitionFile()
	snapshot, err := definition.readSnapshot()
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
	if err := definition.ensureUnchanged(snapshot); err != nil {
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
	return definition.ensureStillMatches(executable, "starting")
}

func (manager *launchdServiceManager) Status(ctx context.Context) (serviceState, error) {
	snapshot, err := manager.definitionFile().readSnapshot()
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

func (manager *launchdServiceManager) RestartIfActive(ctx context.Context, executable string) (bool, error) {
	return restartServiceIfActive(ctx, manager, executable)
}

func (manager *launchdServiceManager) IsDefinitionOutdated() (bool, error) {
	return manager.definitionFile().isOutdated()
}

func (manager *launchdServiceManager) restartRunning(ctx context.Context) error {
	return manager.run(ctx, "kickstart", "-k", manager.target())
}

func (manager *launchdServiceManager) Disable(ctx context.Context) (removed bool, result error) {
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
	loaded, err := manager.isLoaded(ctx)
	if err != nil {
		return false, err
	}
	if loaded {
		if err := manager.run(ctx, "bootout", manager.target()); err != nil {
			return false, err
		}
	}
	if err := definition.ensureUnchanged(snapshot); err != nil {
		return false, err
	}
	if err := manager.files.Remove(manager.plistPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove %s: %w", manager.plistPath(), err)
	}
	return true, nil
}

func (manager *launchdServiceManager) acquireOperationLock() (func() error, error) {
	return acquireServiceOperationLock(manager.lock)
}

func (manager *launchdServiceManager) waitUntilReady(ctx context.Context) error {
	if manager.waitReady == nil {
		return errServiceReadinessUnavailable
	}
	return manager.waitReady(ctx, manager)
}

func (manager *launchdServiceManager) isLoaded(ctx context.Context) (bool, error) {
	loaded, _, err := manager.inspectJob(ctx)
	return loaded, err
}

// mainPID は、launchd が報告する job の main PID を返す。job が無い、または読めなければ 0。
func (manager *launchdServiceManager) mainPID(ctx context.Context) int {
	_, pid, err := manager.inspectJob(ctx)
	if err != nil {
		return 0
	}
	return pid
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

func launchdServiceNotFound(result serviceCommandResult) bool {
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

func launchctlExitError(arguments []string, result serviceCommandResult) error {
	return serviceToolExitError("launchctl", arguments, result)
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
	// KeepAliveはsystemdのRestart=on-failureとそろえ、0以外で終わったときだけ再起動する。
	// engineは`sshc engine --replace`による置き換えとSIGTERMで0で終わる。無条件に再起動
	// すると、置き換えで止めたserviceのengineが戻って前面のengineとロックを取り合う。
	// launchdにはSuccessExitStatus=130に当たる指定が無いため、SIGINTによる130は再起動する。
	return launchdPlistMarker +
		"<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n" +
		"<plist version=\"1.0\">\n<dict>\n" +
		"  <key>Label</key>\n  <string>" + xmlText(launchdServiceLabel) + "</string>\n" +
		"  <key>ProgramArguments</key>\n  <array>\n    <string>" + xmlText(executable) + "</string>\n    <string>engine</string>\n  </array>\n" +
		"  <key>WorkingDirectory</key>\n  <string>" + xmlText(home) + "</string>\n" +
		"  <key>RunAtLoad</key>\n  <true/>\n" +
		"  <key>KeepAlive</key>\n  <dict>\n    <key>SuccessfulExit</key>\n    <false/>\n  </dict>\n" +
		"  <key>ThrottleInterval</key>\n  <integer>5</integer>\n" +
		"</dict>\n</plist>\n", nil
}

func xmlText(value string) string {
	var output bytes.Buffer
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

func waitForLaunchdServiceReady(ctx context.Context, manager *launchdServiceManager) error {
	return waitForEngineReady(ctx, manager.home, engineReadiness{
		mainPID:       manager.mainPID,
		failureDetail: manager.failureDetail,
	})
}

// failureDetail は、準備が間に合わなかったときに launchd が報告する job の状態を返す。
func (manager *launchdServiceManager) failureDetail(ctx context.Context) string {
	result, err := manager.runner.Run(ctx, "print", manager.target())
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
