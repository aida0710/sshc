package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

var errUnmanagedServiceUnit = errors.New("the existing sshc.service is not managed by sshc")

type serviceState uint8

const (
	serviceAbsent serviceState = iota
	serviceInactive
	serviceActive
	serviceUnmanaged
)

type engineServiceManager interface {
	Install(context.Context, string) error
	Status(context.Context) (serviceState, error)
	RestartIfActive(context.Context) (bool, error)
	Disable(context.Context) (bool, error)
}

type serviceDependencies struct {
	manager    func(string) (engineServiceManager, error)
	executable func(context.Context) (string, error)
}

func defaultServiceDependencies() serviceDependencies {
	return serviceDependencies{
		manager:    newPlatformServiceManager,
		executable: managedServiceExecutable,
	}
}

// managedServiceExecutable は更新後も同じ場所を指す管理元の安定パスだけを返す。
// source buildや手動copyを推測で自動起動へ登録しない境界はupdateと同じである。
func managedServiceExecutable(ctx context.Context) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find this executable: %w", err)
	}
	found, err := detectInstallation(executable)
	if err != nil {
		return "", fmt.Errorf("inspect this installation: %w", err)
	}
	switch found.manager {
	case managerHomebrew:
		return homebrewManagedExecutable(ctx, found, osUpdateCommands{})
	case managerShell:
		return found.executable, nil
	default:
		return "", fmt.Errorf("%s is not managed by Homebrew or sshc's install.sh", executable)
	}
}

func runService(
	ctx context.Context,
	action string,
	home string,
	stdout, stderr io.Writer,
	dependencies serviceDependencies,
) int {
	manager, err := dependencies.manager(home)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: service is unavailable: %v\n", err)
		return 1
	}

	switch action {
	case "install":
		executable, err := dependencies.executable(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "sshc: choose a stable service executable: %v\n", err)
			return 1
		}
		if err := manager.Install(ctx, executable); err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return 130
			}
			if errors.Is(err, errUnmanagedServiceUnit) {
				fmt.Fprintln(stderr, "sshc: ~/.config/systemd/user/sshc.service already exists and is not managed by sshc")
				fmt.Fprintln(stderr, "sshc: remove or rename it yourself before running `sshc service install`")
			} else {
				fmt.Fprintf(stderr, "sshc: install service: %v\n", err)
			}
			return 1
		}
		fmt.Fprintln(stdout, "sshc: service installed and started; vault is locked")
		fmt.Fprintln(stdout, "sshc: run `sshc vault unlock` from an interactive terminal")
		return 0
	case "status":
		state, err := manager.Status(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return 130
			}
			fmt.Fprintf(stderr, "sshc: inspect service: %v\n", err)
			return 1
		}
		switch state {
		case serviceAbsent:
			fmt.Fprintln(stdout, "sshc: service is not installed")
		case serviceInactive:
			fmt.Fprintln(stdout, "sshc: managed service is installed but inactive")
		case serviceActive:
			fmt.Fprintln(stdout, "sshc: managed service is active")
		case serviceUnmanaged:
			fmt.Fprintln(stderr, "sshc: sshc.service exists but is not managed by sshc")
			return 1
		default:
			fmt.Fprintln(stderr, "sshc: inspect service: unknown service state")
			return 1
		}
		return 0
	case "disable":
		removed, err := manager.Disable(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return 130
			}
			if errors.Is(err, errUnmanagedServiceUnit) {
				fmt.Fprintln(stderr, "sshc: refusing to remove sshc.service because it is not managed by sshc")
			} else {
				fmt.Fprintf(stderr, "sshc: disable service: %v\n", err)
			}
			return 1
		}
		if removed {
			fmt.Fprintln(stdout, "sshc: service stopped, disabled, and removed")
		} else {
			fmt.Fprintln(stdout, "sshc: service is not installed; nothing changed")
		}
		return 0
	default:
		fmt.Fprintln(stderr, "sshc: invalid service action")
		return 2
	}
}
