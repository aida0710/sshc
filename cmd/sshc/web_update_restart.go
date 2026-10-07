package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"sshc/internal/filelock"
	"sshc/internal/selfupdate"
)

// webUpdateRestartEnvironment selects the private restart helper on the fixed
// `update --yes` invocation. Its job is read from this user's private state.
const webUpdateRestartEnvironment = "SSHC_WEB_UPDATE_RESTART"

func launchWebUpdateRestart(executable, id string) error {
	return startDetachedUpdateProcess(executable, []string{"update", "--yes"}, replaceEnvironment(os.Environ(), map[string]string{webUpdateRestartEnvironment: id}))
}

func startDetachedUpdateProcess(executable string, arguments, environment []string) error {
	command := exec.Command(executable, arguments...)
	command.Env = environment
	configureDetachedUpdateProcess(command)
	if err := command.Start(); err != nil {
		return err
	}
	// Reap while the parent lives. A restart can end the parent first, in which
	// case the OS adopts the helper. It inherits no HTTP sockets or output pipes.
	go func() { _ = command.Wait() }()
	return nil
}

// runWebUpdateRestart lives outside the engine so its own HTTP shutdown cannot
// cancel the restart. Recovery is determined by the new engine's version, since
// service managers can stop this helper after accepting their restart job.
func runWebUpdateRestart(paths userPaths, id string) int {
	ctx, cancel := context.WithTimeout(context.Background(), selfupdate.RestartTimeout)
	defer cancel()
	store := selfupdate.Store{Path: filepath.Join(paths.stateDir, selfupdate.StateFileName)}
	release, err := filelock.TryAcquire(store.Path + selfupdate.RestartLockSuffix)
	if err != nil {
		return exitFailure
	}
	defer release()
	job, err := store.Read()
	if err != nil || job.ID != id || job.State != selfupdate.JobRestarting || job.Target != version {
		return exitFailure
	}
	dependencies := defaultUpdateDependencies()
	err = verifyWebUpdateRestartInstallation(ctx, job, dependencies)
	if err == nil {
		err = restartWebUpdateEngine(ctx, webUpdateRestartRun{paths: paths, job: job, restartService: dependencies.restartService, startProcess: startDetachedUpdateProcess})
	}
	if err != nil {
		_, _ = store.Change(func(stored *selfupdate.Job) error {
			if stored.ID != id || stored.State != selfupdate.JobRestarting {
				return selfupdate.ErrChanged
			}
			stored.State, stored.Problem = selfupdate.JobRestartRequired, "update_restart_failed"
			stored.UpdatedAt = time.Now().UTC()
			return nil
		})
		return exitFailure
	}
	return 0
}

func verifyWebUpdateRestartInstallation(ctx context.Context, job selfupdate.Job, dependencies updateDependencies) error {
	executable, err := dependencies.executable()
	if err != nil {
		return err
	}
	found, err := dependencies.detect(executable)
	if err != nil {
		return err
	}
	managed, err := dependencies.serviceExecutable(ctx, found)
	if err != nil {
		return err
	}
	if managed != job.Plan.Installation.Executable {
		return selfupdate.ErrChanged
	}
	return nil
}

type webUpdateRestartRun struct {
	paths          userPaths
	job            selfupdate.Job
	restartService func(context.Context, string) (bool, error)
	startProcess   func(string, []string, []string) error
}

func restartWebUpdateEngine(ctx context.Context, run webUpdateRestartRun) error {
	paths, job := run.paths, run.job
	client := &http.Client{Timeout: connectTimeout}
	found, err := verifiedHandoff(ctx, paths.stateDir, client)
	if err != nil {
		return err
	}
	if found.PID != job.OwnerPID {
		return selfupdate.ErrChanged
	}
	restarted, err := run.restartService(ctx, job.Plan.Installation.Executable)
	if err != nil || restarted {
		return err
	}
	engineURL, err := url.Parse(found.URL)
	if err != nil || engineURL.Port() == "" {
		return selfupdate.ErrChanged
	}
	// Target the exact proved handoff. A concurrent replacement must never be
	// stopped just because it shares the state directory.
	if err := stopRunningEngine(ctx, paths.stateDir, found, client, lockEngineStart); err != nil {
		return err
	}
	environment := replaceEnvironment(os.Environ(), map[string]string{webUpdateRestartEnvironment: ""})
	// Keep this browser's origin even when --port overrode the saved engine settings.
	return run.startProcess(job.Plan.Installation.Executable, []string{"engine", "--port", engineURL.Port()}, environment)
}
