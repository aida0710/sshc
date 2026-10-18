package main

import (
	"context"
	"fmt"
	"runtime"
)

func runStop(ctx context.Context, environment commandEnvironment) int {
	if ctx.Err() != nil {
		return reportEngineUnreachable(ctx, ctx.Err(), environment.stderr)
	}
	running, err := isEngineRunning(environment.stateDir)
	if err != nil {
		return reportEngineUnreachable(ctx, err, environment.stderr)
	}
	if !running {
		fmt.Fprintln(environment.stdout, "sshc: no sshc engine is running")
		return 0
	}
	found, err := waitForReadyEngine(ctx, environment.stateDir, engineReadiness{timeout: backgroundReadyTimeout, client: environment.client})
	if err != nil {
		return reportEngineUnreachable(ctx, err, environment.stderr)
	}
	if err := stopRunningEngine(ctx, environment.stateDir, found, environment.client, lockEngineStart); err != nil {
		return reportEngineUnreachable(ctx, err, environment.stderr)
	}
	fmt.Fprintln(environment.stdout, "sshc: stopped the sshc engine")
	reportServiceLoginReminder(ctx, environment, newPlatformServiceManager)
	return 0
}

func reportServiceLoginReminder(ctx context.Context, environment commandEnvironment, managerForHome func(string) (engineServiceManager, error)) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return
	}
	manager, err := managerForHome(environment.home)
	if err != nil {
		return
	}
	serviceCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	enabled, err := manager.StartsAtLogin(serviceCtx)
	if err == nil && enabled {
		fmt.Fprintln(environment.stderr, "sshc: the installed service will start the engine at your next login; run sshc service disable to keep it stopped")
	}
}
