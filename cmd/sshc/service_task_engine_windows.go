//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/sys/windows"
)

type taskEngine struct {
	stateDir string
	client   *http.Client
}

func (engine taskEngine) readiness(executable string) engineReadiness {
	return engineReadiness{client: engine.client, verifyProcess: func(pid int) error { return verifyTaskEngineExecutable(pid, executable) }}
}
func (engine taskEngine) probe(ctx context.Context, executable string) error {
	_, err := engine.readiness(executable).probe(ctx, engine.stateDir, engine.client)
	return err
}
func (engine taskEngine) wait(ctx context.Context, executable string) error {
	_, err := waitForReadyEngine(ctx, engine.stateDir, engine.readiness(executable))
	return err
}
func (engine taskEngine) stop(ctx context.Context, executable string) error {
	running, err := isEngineRunning(engine.stateDir)
	if err != nil || !running {
		return err
	}
	found, err := waitForReadyEngine(ctx, engine.stateDir, engine.readiness(executable))
	if err != nil {
		return err
	}
	return stopRunningEngine(ctx, engine.stateDir, found, engine.client, lockEngineStart)
}

func verifyTaskEngineExecutable(pid int, executable string) error {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("inspect the scheduled engine process: %w", err)
	}
	defer windows.CloseHandle(process)
	// Windows extended paths can contain up to 32767 UTF-16 characters.
	const maximumWindowsPath = 32768
	buffer := make([]uint16, maximumWindowsPath)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return fmt.Errorf("read the scheduled engine executable: %w", err)
	}
	actual := windows.UTF16ToString(buffer[:size])
	// Failure to inspect is not readiness: a handoff from another installation must
	// not authorize stopping that process or declaring this task successfully started.
	if !sameTaskExecutable(actual, executable) {
		return errEngineIdentityMismatch
	}
	return nil
}
