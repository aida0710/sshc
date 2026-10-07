package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

var errUnmanagedServiceUnit = errors.New("the existing service definition is not managed by sshc")

// outdatedServiceDefinitionAdvice は、以前の版の sshc が書いた service の定義を、今の版で
// 書き直すよう案内する。sshc update はその定義の service を再起動しない。
const outdatedServiceDefinitionAdvice = "the service definition is from an older sshc; run `sshc service install` to update it"

// errOutdatedServiceDefinition は、sshc が書いた service の定義が以前の版の形のままで、
// 今の版の定義と一致しないことを表す。
var errOutdatedServiceDefinition = errors.New("the service definition is from an older sshc")

const serviceReadyTimeout = 5 * time.Second

type serviceState uint8

const (
	serviceAbsent serviceState = iota
	serviceInactive
	serviceActive
	serviceUnmanaged
)

type engineServiceManager interface {
	InstallPlan(string) (string, error)
	Install(context.Context, string) error
	Status(context.Context) (serviceState, error)
	RestartPlan(string) (string, error)
	RestartIfActive(context.Context, string) (bool, error)
	// IsDefinitionOutdated は、sshc が書いた定義が以前の版の形のままかを返す。
	IsDefinitionOutdated() (bool, error)
	Disable(context.Context) (bool, error)
	DisablePlan() string
}

type serviceDependencies struct {
	manager    func(string) (engineServiceManager, error)
	executable func(context.Context) (string, error)
	confirm    actionConfirmer
	// engineStatus は、起動した engine の Vault の状態を読み、次にすることを選ぶために使う。
	engineStatus engineStatusReader
}

func defaultServiceDependencies() serviceDependencies {
	return serviceDependencies{
		manager:      newPlatformServiceManager,
		executable:   managedServiceExecutable,
		confirm:      systemActionConfirmer,
		engineStatus: readEngineStatus,
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
	return managedInstallationExecutable(ctx, found, systemInstallationCommands{})
}

// serviceRun は、`sshc service <action>` の 1 回の実行である。
type serviceRun struct {
	// action は install、status、restart、disable のどれかである。
	action string
	// yes は、変更の確認を省く。
	yes          bool
	home         string
	stdout       io.Writer
	stderr       io.Writer
	dependencies serviceDependencies
}

func runService(ctx context.Context, run serviceRun) int {
	manager, err := run.dependencies.manager(run.home)
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: service is unavailable: %v\n", err)
		return exitFailure
	}
	switch run.action {
	case "install":
		return run.install(ctx, manager)
	case "status":
		return run.status(ctx, manager)
	case "restart":
		return run.restart(ctx, manager)
	case "disable":
		return run.disable(ctx, manager)
	default:
		fmt.Fprintln(run.stderr, "sshc: invalid service action")
		return exitUsage
	}
}

// install は、安定したパスの実行ファイルを service に登録して起動する。
func (run serviceRun) install(ctx context.Context, manager engineServiceManager) int {
	executable, err := run.dependencies.executable(ctx)
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: choose a stable service executable: %v\n", err)
		return exitFailure
	}
	plan, err := manager.InstallPlan(executable)
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: plan service installation: %v\n", err)
		return exitFailure
	}
	fmt.Fprintf(run.stdout, "sshc: %s\n", plan)
	if confirmed, code := run.confirm(ctx); !confirmed {
		return code
	}
	if err := manager.Install(ctx, executable); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		if errors.Is(err, errUnmanagedServiceUnit) {
			fmt.Fprintln(run.stderr, "sshc: a service definition already exists and is not managed by sshc")
			fmt.Fprintln(run.stderr, "sshc: move or remove it yourself before running `sshc service install`")
		} else {
			fmt.Fprintf(run.stderr, "sshc: install service: %v\n", err)
		}
		return exitFailure
	}
	fmt.Fprintln(run.stdout, "sshc: service installed and started")
	if advice := vaultNextStep(ctx, run.dependencies.engineStatus, run.home); advice != "" {
		fmt.Fprintf(run.stdout, "sshc: %s\n", advice)
	}
	return 0
}

// status は、service が登録されているか、動いているかを出す。
func (run serviceRun) status(ctx context.Context, manager engineServiceManager) int {
	state, err := manager.Status(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		fmt.Fprintf(run.stderr, "sshc: inspect service: %v\n", err)
		return exitFailure
	}
	switch state {
	case serviceAbsent:
		fmt.Fprintln(run.stdout, "sshc: service is not installed")
		return 0
	case serviceInactive:
		fmt.Fprintln(run.stdout, "sshc: managed service is installed but inactive")
	case serviceActive:
		fmt.Fprintln(run.stdout, "sshc: managed service is active")
	case serviceUnmanaged:
		fmt.Fprintln(run.stderr, "sshc: a service definition exists but is not managed by sshc")
		return exitFailure
	default:
		fmt.Fprintln(run.stderr, "sshc: inspect service: unknown service state")
		return exitFailure
	}
	outdated, err := manager.IsDefinitionOutdated()
	if err != nil {
		fmt.Fprintf(run.stderr, "sshc: inspect service: %v\n", err)
		return exitFailure
	}
	if outdated {
		fmt.Fprintf(run.stdout, "sshc: %s\n", outdatedServiceDefinitionAdvice)
	}
	return 0
}

// disable は、sshc が登録した service を止めて、登録を消す。sshc が登録したので
// ない定義には触らない。
func (run serviceRun) disable(ctx context.Context, manager engineServiceManager) int {
	state, err := manager.Status(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		if errors.Is(err, errUnmanagedServiceUnit) || state == serviceUnmanaged {
			fmt.Fprintln(run.stderr, "sshc: refusing to remove the service definition because it is not managed by sshc")
		} else {
			fmt.Fprintf(run.stderr, "sshc: inspect service before disabling it: %v\n", err)
		}
		return exitFailure
	}
	if state == serviceAbsent {
		fmt.Fprintln(run.stdout, "sshc: service is not installed; nothing changed")
		return 0
	}
	if state == serviceUnmanaged {
		fmt.Fprintln(run.stderr, "sshc: refusing to remove the service definition because it is not managed by sshc")
		return exitFailure
	}
	fmt.Fprintf(run.stdout, "sshc: %s\n", manager.DisablePlan())
	if confirmed, code := run.confirm(ctx); !confirmed {
		return code
	}
	removed, err := manager.Disable(ctx)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return exitInterrupted
		}
		if errors.Is(err, errUnmanagedServiceUnit) {
			fmt.Fprintln(run.stderr, "sshc: refusing to remove the service definition because it is not managed by sshc")
		} else {
			fmt.Fprintf(run.stderr, "sshc: disable service: %v\n", err)
		}
		return exitFailure
	}
	if removed {
		fmt.Fprintln(run.stdout, "sshc: service stopped, disabled, and removed")
	} else {
		fmt.Fprintln(run.stdout, "sshc: service is not installed; nothing changed")
	}
	return 0
}

// confirm は、出した計画を進めてよいかを尋ねる。
func (run serviceRun) confirm(ctx context.Context) (bool, int) {
	return confirmChange(ctx, changeConfirmation{
		yes: run.yes, confirmer: run.dependencies.confirm, stdout: run.stdout, stderr: run.stderr,
	})
}
