package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// Task Scheduler's TASK_STATE values, independent of the Windows display language.
type scheduledTaskState uint32

const (
	taskUnknown scheduledTaskState = iota
	taskDisabled
	taskQueued
	taskReady
	taskRunning
)

type serviceTaskSnapshot struct {
	exists     bool
	state      scheduledTaskState
	definition taskElement
}

type taskServiceManager struct {
	sid              string
	runner           serviceCommandRunner
	readTask         func(context.Context) (serviceTaskSnapshot, error)
	engineRunning    func() (bool, error)
	engineResponding func(context.Context, string) error
	stopEngine       func(context.Context, string) error
	waitReady        func(context.Context, string) error
	lock             func() (func() error, error)
}

func (manager *taskServiceManager) InstallPlan(executable string) (string, error) {
	if _, err := serviceTaskXML(executable, manager.sid); err != nil {
		return "", err
	}
	return fmt.Sprintf("register the sshc logon task in Task Scheduler and start %s engine --background; administrator rights are not required", executable), nil
}
func (manager *taskServiceManager) DisablePlan() string {
	return "stop the sshc engine and delete the sshc logon task from Task Scheduler"
}
func (manager *taskServiceManager) acquireOperationLock() (func() error, error) {
	return acquireServiceOperationLock(manager.lock)
}

func (manager *taskServiceManager) Install(ctx context.Context, executable string) (result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readTask(ctx)
	if err != nil {
		return err
	}
	if snapshot.exists && !snapshot.definition.managedBy(manager.sid) {
		return errUnmanagedServiceUnit
	}
	running, err := manager.engineRunning()
	if err != nil {
		return err
	}
	if running || snapshot.exists && (snapshot.state == taskRunning || snapshot.state == taskQueued) {
		return errors.New("an sshc engine or task is already running; run `sshc stop` and retry `sshc service install`")
	}
	definition, err := serviceTaskXML(executable, manager.sid)
	if err != nil {
		return err
	}
	staged, err := os.CreateTemp("", "sshc-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	_, writeErr := staged.Write(taskUTF16(definition))
	if err := errors.Join(writeErr, staged.Close()); err != nil {
		return err
	}
	if err := manager.ensureUnchanged(ctx, snapshot); err != nil {
		return err
	}
	arguments := []string{"/Create", "/TN", serviceTaskName, "/XML", staged.Name()}
	// /F is allowed only after recognizing and rechecking an existing managed task.
	// A concurrently created task must cause creation to fail instead of being replaced.
	if snapshot.exists {
		arguments = append(arguments, "/F")
	}
	if err := manager.run(ctx, arguments...); err != nil {
		return err
	}
	if err := manager.taskDefinition(ctx).ensureStillMatches(executable, "registering"); err != nil {
		return err
	}
	if err := manager.startTask(ctx); err != nil {
		return err
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return fmt.Errorf("service did not become ready: %w", err)
	}
	return manager.taskDefinition(ctx).ensureStillMatches(executable, "starting")
}

func (manager *taskServiceManager) Status(ctx context.Context) (serviceState, error) {
	snapshot, err := manager.readTask(ctx)
	if err != nil || !snapshot.exists {
		return serviceAbsent, err
	}
	if !snapshot.definition.managedBy(manager.sid) {
		return serviceUnmanaged, nil
	}
	if snapshot.state == taskRunning || snapshot.state == taskQueued {
		return serviceActive, nil
	}
	return serviceInactive, nil
}
func (manager *taskServiceManager) StatusDetail(ctx context.Context) (string, error) {
	snapshot, err := manager.managedTask(ctx)
	if err != nil {
		return "", err
	}
	states := map[scheduledTaskState]string{taskUnknown: "Unknown", taskDisabled: "Disabled", taskQueued: "Queued", taskReady: "Ready", taskRunning: "Running"}
	response := "not responding"
	if err := manager.engineResponding(ctx, snapshot.definition.executable()); err == nil {
		response = "responding"
	}
	return fmt.Sprintf("Task Scheduler: %s; engine: %s", states[snapshot.state], response), nil
}
func (manager *taskServiceManager) StartsAtLogin(ctx context.Context) (bool, error) {
	snapshot, err := manager.readTask(ctx)
	if err != nil || !snapshot.exists || !snapshot.definition.managedBy(manager.sid) {
		return false, err
	}
	task := snapshot.definition
	return snapshot.state != taskDisabled && task.at("Settings", "Enabled").Text != "false" &&
		task.at("Triggers", "LogonTrigger", "UserId").Text == manager.sid && task.at("Triggers", "LogonTrigger", "Enabled").Text != "false", nil
}
func (manager *taskServiceManager) IsDefinitionOutdated() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceReadyTimeout)
	defer cancel()
	snapshot, err := manager.readTask(ctx)
	if err != nil || !snapshot.exists || !snapshot.definition.managedBy(manager.sid) {
		return false, err
	}
	return !snapshot.definition.matches(snapshot.definition.executable(), manager.sid), nil
}
func (manager *taskServiceManager) RestartPlan(executable string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceReadyTimeout)
	defer cancel()
	snapshot, err := manager.managedTask(ctx)
	if err != nil {
		return "", err
	}
	if !snapshot.definition.matches(snapshot.definition.executable(), manager.sid) {
		return "", errOutdatedServiceDefinition
	}
	if !sameTaskExecutable(snapshot.definition.executable(), executable) {
		return "", errServiceExecutableMismatch
	}
	return fmt.Sprintf("restart the active sshc logon task using %s; existing connections and transfers will be interrupted", executable), nil
}
func (manager *taskServiceManager) RestartIfActive(ctx context.Context, executable string) (bool, error) {
	return restartServiceIfActive(ctx, manager, executable)
}

// startIfEnabled resumes an existing task after the web helper stopped its engine.
// It never rewrites a definition or enables a disabled task.
func (manager *taskServiceManager) startIfEnabled(ctx context.Context, executable string) (started bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readTask(ctx)
	if err != nil || !snapshot.exists {
		return false, err
	}
	if !snapshot.definition.managedBy(manager.sid) {
		return false, errUnmanagedServiceUnit
	}
	if snapshot.state == taskDisabled || snapshot.definition.at("Settings", "Enabled").Text == "false" {
		return false, nil
	}
	if !snapshot.definition.matches(executable, manager.sid) {
		return false, errServiceExecutableMismatch
	}
	if err := manager.startStoppedTask(ctx, snapshot); err != nil {
		return false, err
	}
	if err := manager.waitUntilReady(ctx); err != nil {
		return false, err
	}
	return true, manager.taskDefinition(ctx).ensureStillMatches(executable, "starting")
}
func (manager *taskServiceManager) restartRunning(ctx context.Context) error {
	snapshot, err := manager.managedTask(ctx)
	if err != nil {
		return err
	}
	if snapshot.state != taskRunning && snapshot.state != taskQueued {
		return nil
	}
	if err := manager.stopEngine(ctx, snapshot.definition.executable()); err != nil {
		return err
	}
	return manager.startStoppedTask(ctx, snapshot)
}

// Wait for Scheduler to observe process exit, then recheck the definition before starting it.
func (manager *taskServiceManager) startStoppedTask(ctx context.Context, snapshot serviceTaskSnapshot) error {
	if err := manager.waitTaskStopped(ctx); err != nil {
		return err
	}
	if err := manager.ensureUnchanged(ctx, snapshot); err != nil {
		return err
	}
	return manager.startTask(ctx)
}
func (manager *taskServiceManager) waitUntilReady(ctx context.Context) error {
	if manager.waitReady == nil {
		return errServiceReadinessUnavailable
	}
	snapshot, err := manager.managedTask(ctx)
	if err != nil {
		return err
	}
	return manager.waitReady(ctx, snapshot.definition.executable())
}
func (manager *taskServiceManager) Disable(ctx context.Context) (removed bool, result error) {
	release, err := manager.acquireOperationLock()
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, release()) }()
	snapshot, err := manager.readTask(ctx)
	if err != nil || !snapshot.exists {
		return false, err
	}
	if !snapshot.definition.managedBy(manager.sid) {
		return false, errUnmanagedServiceUnit
	}
	if snapshot.state == taskRunning || snapshot.state == taskQueued {
		if err := manager.stopEngine(ctx, snapshot.definition.executable()); err != nil {
			return false, err
		}
		if err := manager.waitTaskStopped(ctx); err != nil {
			return false, err
		}
	}
	if err := manager.ensureUnchanged(ctx, snapshot); err != nil {
		return false, err
	}
	if err := manager.run(ctx, "/Delete", "/TN", serviceTaskName, "/F"); err != nil {
		return false, err
	}
	return true, nil
}
func (manager *taskServiceManager) ensureUnchanged(ctx context.Context, expected serviceTaskSnapshot) error {
	actual, err := manager.readTask(ctx)
	if err != nil {
		return err
	}
	if actual.exists != expected.exists || canonicalTask(actual.definition, "") != canonicalTask(expected.definition, "") {
		return errors.New("sshc task changed during the operation; it was left in place")
	}
	return nil
}
func (manager *taskServiceManager) run(ctx context.Context, arguments ...string) error {
	answer, err := manager.runner.Run(ctx, arguments...)
	if err != nil {
		return err
	}
	if answer.ExitCode != 0 {
		return serviceToolExitError("schtasks.exe", arguments, answer)
	}
	return nil
}
func (manager *taskServiceManager) startTask(ctx context.Context) error {
	if err := manager.run(ctx, "/Run", "/TN", serviceTaskName); err != nil {
		return err
	}
	return manager.waitTaskState(ctx, true)
}
func (manager *taskServiceManager) waitTaskStopped(ctx context.Context) error {
	return manager.waitTaskState(ctx, false)
}
func (manager *taskServiceManager) waitTaskState(ctx context.Context, running bool) error {
	waitCtx, cancel := context.WithTimeout(ctx, serviceReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(engineReadyPollInterval)
	defer ticker.Stop()
	for {
		snapshot, err := manager.managedTask(waitCtx)
		if err != nil {
			return err
		}
		if running && snapshot.state == taskRunning || !running && snapshot.state != taskRunning && snapshot.state != taskQueued {
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for Task Scheduler state: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (manager *taskServiceManager) managedTask(ctx context.Context) (serviceTaskSnapshot, error) {
	snapshot, err := manager.readTask(ctx)
	if err != nil {
		return snapshot, err
	}
	if !snapshot.exists {
		return snapshot, errServiceNotInstalled
	}
	if !snapshot.definition.managedBy(manager.sid) {
		return snapshot, errUnmanagedServiceUnit
	}
	return snapshot, nil
}
