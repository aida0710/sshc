package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var (
	errServiceNotInstalled       = errors.New("service is not installed; run `sshc service install` to install and start it")
	errServiceExecutableMismatch = errors.New("the service definition uses a different sshc executable")
)

// restart は、稼働中のサービスだけを対象にする。停止中のサービスを起動したり、
// 別の導入を登録し直したりする場合は、install の計画と確認を経由する。
func (run serviceRun) restart(ctx context.Context, manager engineServiceManager) int {
	state, err := manager.Status(ctx)
	if err != nil {
		return writeServiceRestartFailure(ctx, run.stderr, err)
	}
	switch state {
	case serviceAbsent:
		return writeServiceRestartFailure(ctx, run.stderr, errServiceNotInstalled)
	case serviceInactive:
		return writeServiceRestartFailure(ctx, run.stderr, errors.New("managed service is installed but inactive; run `sshc service install` to start it"))
	case serviceUnmanaged:
		return writeServiceRestartFailure(ctx, run.stderr, errUnmanagedServiceUnit)
	case serviceActive:
	default:
		return writeServiceRestartFailure(ctx, run.stderr, errors.New("unknown service state"))
	}
	executable, err := run.dependencies.executable(ctx)
	if err != nil {
		return writeServiceRestartFailure(ctx, run.stderr, fmt.Errorf("choose a stable service executable: %w", err))
	}
	plan, err := manager.RestartPlan(executable)
	if err != nil {
		return writeServiceRestartFailure(ctx, run.stderr, err)
	}
	fmt.Fprintf(run.stdout, "sshc: %s\n", plan)
	if confirmed, code := run.confirm(ctx); !confirmed {
		return code
	}
	// 確認待ちの間にも状態は変わり得る。定義と稼働状態をlock内で再検査し、
	// PID・handoff・status APIがそろうまで待つ処理をupdateと共有する。
	restarted, err := manager.RestartIfActive(ctx, executable)
	if err != nil {
		return writeServiceRestartFailure(ctx, run.stderr, err)
	}
	if !restarted {
		return writeServiceRestartFailure(ctx, run.stderr, errors.New("service was not restarted because its state or definition changed; run `sshc service status` and retry"))
	}
	fmt.Fprintln(run.stdout, "sshc: managed service restarted")
	if advice := vaultNextStep(ctx, run.dependencies.engineStatus, run.home); advice != "" {
		fmt.Fprintf(run.stdout, "sshc: %s\n", advice)
	}
	return 0
}

func writeServiceRestartFailure(ctx context.Context, stderr io.Writer, err error) int {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return exitInterrupted
	}
	switch {
	case errors.Is(err, errUnmanagedServiceUnit):
		fmt.Fprintln(stderr, "sshc: refusing to restart the service because its definition is not managed by sshc")
	case errors.Is(err, errOutdatedServiceDefinition):
		fmt.Fprintf(stderr, "sshc: %s\n", outdatedServiceDefinitionAdvice)
	case errors.Is(err, errServiceExecutableMismatch):
		fmt.Fprintln(stderr, "sshc: refusing to restart the service because its definition uses a different sshc executable")
		fmt.Fprintln(stderr, "sshc: use the registered installation, or run `sshc service install` to register this installation")
	default:
		fmt.Fprintf(stderr, "sshc: restart service: %v\n", err)
	}
	return exitFailure
}
