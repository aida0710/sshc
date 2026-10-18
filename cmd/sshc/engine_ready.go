package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"sshc/internal/app"
	"sshc/internal/handoff"
)

// These bounds keep readiness responsive without busy polling or letting one
// unresponsive handoff consume the whole startup deadline.
const (
	engineReadyPollInterval = 50 * time.Millisecond
	engineReadyProbeTimeout = 250 * time.Millisecond
)

type engineReadiness struct {
	// A nil mainPID accepts any verified engine; zero means the manager has no PID yet.
	mainPID       func(context.Context) int
	failureDetail func(context.Context) string
	timeout       time.Duration
	process       *detachedProcess
	client        *http.Client
}

func waitForEngineReady(ctx context.Context, home string, readiness engineReadiness) error {
	stateDir, err := app.StateDir(home)
	if err != nil {
		return err
	}
	_, err = waitForReadyEngine(ctx, stateDir, readiness)
	return err
}

// waitForReadyEngine verifies the PID, challenge and status together. A locked
// Vault is ready: it can be unlocked from the browser after opening the UI.
func waitForReadyEngine(ctx context.Context, stateDir string, readiness engineReadiness) (handoff.Handoff, error) {
	timeout := readiness.timeout
	if timeout == 0 {
		timeout = serviceReadyTimeout
	}
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	readyDeadline, _ := readyCtx.Deadline()
	ticker := time.NewTicker(engineReadyPollInterval)
	defer ticker.Stop()
	client := noRedirectClient(readiness.client)
	if client.Timeout == 0 || client.Timeout > engineReadyProbeTimeout {
		client.Timeout = engineReadyProbeTimeout
	}
	var exited <-chan processExit
	if readiness.process != nil {
		exited = readiness.process.exited
	}
	var childFailure *engineExitedError
	for {
		found, err := readiness.probe(readyCtx, stateDir, client)
		if err == nil {
			return found, nil
		}
		select {
		case <-ctx.Done():
			return handoff.Handoff{}, ctx.Err()
		case <-readyCtx.Done():
			if ctx.Err() != nil {
				return handoff.Handoff{}, ctx.Err()
			}
			// The caller's deadline bounded this wait when it is no later than ours. Its
			// timer can fire a moment after ours, so report the caller's deadline either way.
			if callerDeadline, ok := ctx.Deadline(); ok && !callerDeadline.After(readyDeadline) {
				return handoff.Handoff{}, context.DeadlineExceeded
			}
			if childFailure != nil {
				return handoff.Handoff{}, errors.Join(childFailure, err)
			}
			return handoff.Handoff{}, readiness.timeoutError(ctx, err)
		case exit := <-exited:
			childFailure = &engineExitedError{exit: exit}
			running, err := isEngineRunning(stateDir)
			if err != nil {
				return handoff.Handoff{}, errors.Join(childFailure, err)
			}
			if !running {
				return handoff.Handoff{}, childFailure
			}
			// Another starter won engine.lock. Its handoff may still be pending.
			exited = nil
			readiness.mainPID = nil
		case <-ticker.C:
		}
	}
}

func (readiness engineReadiness) timeoutError(ctx context.Context, probeError error) error {
	if readiness.failureDetail == nil {
		return probeError
	}
	detail := readiness.failureDetail(ctx)
	if detail == "" {
		detail = "engine readiness was not published"
	}
	return fmt.Errorf("%s; stop any manually running `sshc engine` and retry", detail)
}

func (readiness engineReadiness) probe(ctx context.Context, stateDir string, client *http.Client) (handoff.Handoff, error) {
	pid := -1
	if readiness.mainPID != nil {
		pid = readiness.mainPID(ctx)
	}
	if pid == 0 {
		return handoff.Handoff{}, errors.New("engine readiness was not published")
	}
	found, err := verifiedHandoff(ctx, stateDir, client)
	if err != nil {
		return handoff.Handoff{}, err
	}
	if pid > 0 && found.PID != pid {
		return handoff.Handoff{}, errEngineIdentityMismatch
	}
	_, err = requestStatus(ctx, found, client)
	return found, err
}

type engineExitedError struct{ exit processExit }

func (failure *engineExitedError) Error() string {
	return fmt.Sprintf("the background engine exited before it was ready (exit code %d)", failure.exit.code)
}

func (failure *engineExitedError) Unwrap() error { return failure.exit.err }
