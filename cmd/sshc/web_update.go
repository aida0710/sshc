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
	dependencies := defaultUpdateDependencies()
	return selfupdate.New(selfupdate.Dependencies{
		Context: ctx, Current: current, PID: os.Getpid(), StatePath: filepath.Join(paths.stateDir, selfupdate.StateFileName),
		Latest: dependencies.latest,
		Inspect: func(ctx context.Context) (selfupdate.Installation, error) {
			return inspectWebInstallation(ctx, dependencies)
		},
		Install: func(ctx context.Context, plan selfupdate.Plan) error {
			// The receipt or Homebrew ownership must still match immediately before installation.
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
			err = dependencies.install(ctx, found, releasecheck.Release{Version: plan.Target}, io.Discard, io.Discard)
			if errors.Is(err, errHomebrewTapNotRefreshed) {
				return selfupdate.Failure("update_homebrew_refresh_required")
			}
			if errors.Is(err, os.ErrPermission) {
				return selfupdate.ErrPermission
			}
			return err
		},
		Restart: func(ctx context.Context, job selfupdate.Job) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return launchWebUpdateRestart(job.Plan.Installation.Executable, job.ID)
		},
	})
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
	if found.manager == managerUnknown {
		return selfupdate.Installation{}, selfupdate.Failure("update_unmanaged")
	}
	managed, err := dependencies.serviceExecutable(ctx, found)
	if err != nil {
		return selfupdate.Installation{}, selfupdate.Failure("update_unmanaged")
	}
	directories := []string{filepath.Dir(found.executable)}
	manager := "install.sh"
	if found.manager == managerHomebrew {
		manager = "homebrew"
		directories = append(directories, filepath.Dir(filepath.Dir(found.brew)))
	}
	for _, directory := range directories {
		if err := probeUpdateDirectory(directory); err != nil {
			return selfupdate.Installation{}, selfupdate.ErrPermission
		}
	}
	digest, err := fileSHA256(found.executable)
	if err != nil {
		return selfupdate.Installation{}, selfupdate.ErrUnavailable
	}
	return selfupdate.Installation{Manager: manager, Executable: managed, Identity: found.brew + ":" + digest}, nil
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
