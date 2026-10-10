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
	dependencies.Install = func(ctx context.Context, plan selfupdate.Plan) (string, error) {
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

func installWebUpdate(ctx context.Context, plan selfupdate.Plan, dependencies updateDependencies) (string, error) {
	inspected, err := inspectWebUpdateInstallation(ctx, dependencies)
	if err != nil {
		return "", err
	}
	if inspected.evidence != plan.Installation {
		return "", selfupdate.ErrChanged
	}
	installedVersion := plan.Target
	if inspected.found.manager == managerHomebrew {
		if dependencies.upgradeHomebrew == nil {
			return "", selfupdate.ErrUnavailable
		}
		installedVersion, err = dependencies.upgradeHomebrew(ctx, inspected.found)
	} else {
		err = dependencies.install(ctx, inspected.found, releasecheck.Release{Version: plan.Target}, io.Discard, io.Discard)
	}
	if errors.Is(err, os.ErrPermission) {
		return "", selfupdate.ErrPermission
	}
	return installedVersion, err
}

func inspectWebInstallation(ctx context.Context, dependencies updateDependencies) (selfupdate.Installation, error) {
	inspected, err := inspectWebUpdateInstallation(ctx, dependencies)
	return inspected.evidence, err
}

type inspectedWebInstallation struct {
	found    installation
	evidence selfupdate.Installation
}

func inspectWebUpdateInstallation(ctx context.Context, dependencies updateDependencies) (inspectedWebInstallation, error) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		return inspectedWebInstallation{}, selfupdate.Failure("update_" + runtime.GOOS + "_unsupported")
	}
	executable, err := dependencies.executable()
	if err != nil {
		return inspectedWebInstallation{}, selfupdate.ErrUnavailable
	}
	found, err := dependencies.detect(executable)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return inspectedWebInstallation{}, selfupdate.ErrPermission
		}
		return inspectedWebInstallation{}, selfupdate.Failure("update_unmanaged")
	}
	if err := requireWebUpdateManager(found); err != nil {
		return inspectedWebInstallation{}, err
	}
	managed, err := dependencies.serviceExecutable(ctx, found)
	if err != nil {
		return inspectedWebInstallation{}, selfupdate.Failure("update_unmanaged")
	}
	if err := probeUpdateDirectory(filepath.Dir(found.executable)); err != nil {
		return inspectedWebInstallation{}, selfupdate.ErrPermission
	}
	digest, err := fileSHA256(found.executable)
	if err != nil {
		return inspectedWebInstallation{}, selfupdate.ErrUnavailable
	}
	manager := "install.sh"
	if found.manager == managerHomebrew {
		manager = "homebrew"
	}
	return inspectedWebInstallation{found: found, evidence: selfupdate.Installation{
		Manager: manager, Executable: managed, Identity: found.brew + ":" + digest,
	}}, nil
}

func requireWebUpdateManager(found installation) error {
	if found.manager != managerShell && found.manager != managerHomebrew {
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
