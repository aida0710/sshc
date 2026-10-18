package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"sshc/internal/enginelog"
	"sshc/internal/handoff"
)

// Background startup includes first-use storage initialization and antivirus
// scanning on Windows, so allow more time than a managed service restart.
const backgroundReadyTimeout = 20 * time.Second

type backgroundEngineStarter func() (*detachedProcess, error)

func isEngineRunning(stateDir string) (bool, error) {
	release, err := lockEngineStart(stateDir)
	if errors.Is(err, errEngineRunning) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, release()
}

func ensureBackgroundEngine(ctx context.Context, environment commandEnvironment, start backgroundEngineStarter) (handoff.Handoff, error) {
	if err := ctx.Err(); err != nil {
		return handoff.Handoff{}, err
	}
	running, err := isEngineRunning(environment.stateDir)
	if err != nil {
		return handoff.Handoff{}, err
	}
	readyCtx, cancel := context.WithTimeout(ctx, backgroundReadyTimeout)
	defer cancel()
	deadline, _ := readyCtx.Deadline()
	readiness := engineReadiness{client: environment.client}
	if running {
		if found, err := verifiedHandoff(readyCtx, environment.stateDir, environment.client); err == nil {
			return found, nil
		}
	} else {
		process, err := start()
		if err != nil {
			return handoff.Handoff{}, err
		}
		defer process.release()
		fmt.Fprintf(environment.stderr, "sshc: started the sshc engine in the background (log: %s)\n", enginelog.Path(environment.stateDir))
		readiness.process = process
		readiness.mainPID = func(context.Context) int { return process.pid }
	}
	// Include the first proof request and process creation in the same startup budget.
	readiness.timeout = time.Until(deadline)
	found, err := waitForReadyEngine(ctx, environment.stateDir, readiness)
	var exited *engineExitedError
	if errors.As(err, &exited) {
		tail, tailErr := enginelog.Tail(enginelog.Path(environment.stateDir))
		if tailErr != nil {
			return handoff.Handoff{}, fmt.Errorf("%w; cannot read engine log: %v", err, tailErr)
		}
		return handoff.Handoff{}, fmt.Errorf("%w\n%s", err, tail)
	}
	return found, err
}

func startBackgroundEngine() (*detachedProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return startLoggedEngine(engineProcessCommand{executable: executable, arguments: []string{"engine", "--background"}, environment: os.Environ()})
}

type engineProcessCommand struct {
	executable  string
	arguments   []string
	environment []string
}

func startLoggedEngine(invocation engineProcessCommand) (*detachedProcess, error) {
	// The engine opens its own log; the launcher inherits no log handles.
	return startDetachedProcess(func() *exec.Cmd {
		command := exec.Command(invocation.executable, invocation.arguments...)
		command.Env = invocation.environment
		return command
	})
}
