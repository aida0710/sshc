package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
)

func newWebUpdater(ctx context.Context, paths userPaths, current string) *selfupdate.Service {
	return selfupdate.New(webUpdateDependencies(selfupdate.Dependencies{
		Context: ctx, Current: current, PID: os.Getpid(), StatePath: filepath.Join(paths.stateDir, selfupdate.StateFileName),
	}, defaultUpdateDependencies()))
}

func webUpdateDependencies(dependencies selfupdate.Dependencies, installationDependencies updateDependencies) selfupdate.Dependencies {
	dependencies.Latest = installationDependencies.latest
	dependencies.Inspect = func(ctx context.Context) (selfupdate.Installation, error) {
		return inspectWebInstallation(ctx, installationDependencies)
	}
	dependencies.Install = func(ctx context.Context, plan selfupdate.Plan) error {
		return installWebUpdate(ctx, plan, installationDependencies)
	}
	dependencies.Restart = func(ctx context.Context, job selfupdate.Job) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return launchWebUpdateRestart(job.Plan.Installation.Executable, job.ID)
	}
	return dependencies
}

func installWebUpdate(ctx context.Context, plan selfupdate.Plan, dependencies updateDependencies) error {
	// The receipt must still match immediately before installation.
	inspected, err := inspectWebInstallation(ctx, dependencies)
	if err != nil {
		return err
	}
	if inspected != plan.Installation {
		return selfupdate.ErrChanged
	}
	executable, err := dependencies.executable()
	if err != nil {
		return err
	}
	found, err := dependencies.detect(executable)
	if err != nil {
		return err
	}
	if err := requireWebUpdateManager(found); err != nil {
		return err
	}
	err = dependencies.install(ctx, found, releasecheck.Release{Version: plan.Target}, io.Discard, io.Discard)
	if errors.Is(err, os.ErrPermission) {
		return selfupdate.ErrPermission
	}
	return err
}

func inspectWebInstallation(ctx context.Context, dependencies updateDependencies) (selfupdate.Installation, error) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		return selfupdate.Installation{}, selfupdate.Failure("update_" + runtime.GOOS + "_unsupported")
	}
	executable, err := dependencies.executable()
	if err != nil {
		return selfupdate.Installation{}, selfupdate.ErrUnavailable
	}
	found, err := dependencies.detect(executable)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return selfupdate.Installation{}, selfupdate.ErrPermission
		}
		return selfupdate.Installation{}, selfupdate.Failure("update_unmanaged")
	}
	if err := requireWebUpdateManager(found); err != nil {
		return selfupdate.Installation{}, err
	}
	managed, err := dependencies.serviceExecutable(ctx, found)
	if err != nil {
		return selfupdate.Installation{}, selfupdate.Failure("update_unmanaged")
	}
	if err := probeUpdateDirectory(filepath.Dir(found.executable)); err != nil {
		return selfupdate.Installation{}, selfupdate.ErrPermission
	}
	digest, err := fileSHA256(found.executable)
	if err != nil {
		return selfupdate.Installation{}, selfupdate.ErrUnavailable
	}
	return selfupdate.Installation{Manager: "install.sh", Executable: managed, Identity: found.brew + ":" + digest}, nil
}

func requireWebUpdateManager(found installation) error {
	// brew upgrade follows the current formula and cannot pin the confirmed tag.
	if found.manager == managerHomebrew {
		return selfupdate.Failure("update_homebrew_unsupported")
	}
	if found.manager != managerShell {
		return selfupdate.Failure("update_unmanaged")
	}
	return nil
}

// Creating and removing a private file checks ACLs and read-only filesystems,
// which mode bits alone cannot establish. No installed file is changed.
func probeUpdateDirectory(directory string) error {
	probe, err := os.CreateTemp(directory, ".sshc-update-permission-*")
	if err != nil {
		return err
	}
	return errors.Join(probe.Close(), os.Remove(probe.Name()))
}
