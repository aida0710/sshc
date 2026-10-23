package main

import (
	"context"
	"fmt"
	"io"
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
	process, err := startDetachedProcess(func() *exec.Cmd {
		command := exec.Command(executable, arguments...)
		command.Env = environment
		return command
	})
	if err != nil {
		return err
	}
	process.release()
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
	if err != nil || job.ID != id || job.State != selfupdate.JobRestarting || job.RestartVersion() != version {
		return exitFailure
	}
	// 経過を書けなくても再起動は続ける。画面に経過が出ないだけである。
	output := selfupdate.OpenLog(store.Path, id)
	defer output.Close()
	dependencies := defaultUpdateDependencies()
	err = verifyWebUpdateRestartInstallation(ctx, job, dependencies)
	if err == nil {
		err = restartWebUpdateEngine(ctx, webUpdateRestartRun{paths: paths, job: job, output: output, restartService: dependencies.restartService,
			startService: webUpdateServiceStarter(),
			client:       &http.Client{Timeout: connectTimeout},
			startProcess: func(executable string, arguments, environment []string) error {
				process, err := startLoggedEngine(engineProcessCommand{executable: executable, arguments: arguments, environment: environment})
				if err == nil {
					process.release()
				}
				return err
			},
		})
	}
	if err != nil {
		fmt.Fprintf(output, "sshc: the engine could not be restarted: %v\n", err)
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
	client *http.Client
	paths  userPaths
	job    selfupdate.Job
	// output は、ジョブのログである。画面がインストーラーの出力に続けて、再起動の経過を表示する。
	output         io.Writer
	restartService func(context.Context, string) (bool, error)
	startService   func(context.Context, string) (bool, error)
	startProcess   func(string, []string, []string) error
}

func restartWebUpdateEngine(ctx context.Context, run webUpdateRestartRun) error {
	paths, job := run.paths, run.job
	client := run.client
	found, err := verifiedHandoff(ctx, paths.stateDir, client)
	if err != nil {
		return err
	}
	if found.PID != job.OwnerPID {
		return selfupdate.ErrChanged
	}
	if run.startService == nil {
		fmt.Fprintln(run.output, "sshc: restarting the sshc service")
		restarted, err := run.restartService(ctx, job.Plan.Installation.Executable)
		if err != nil || restarted {
			return err
		}
	}
	engineURL, err := url.Parse(found.URL)
	if err != nil || engineURL.Port() == "" {
		return selfupdate.ErrChanged
	}
	// Target the exact proved handoff. A concurrent replacement must never be
	// stopped just because it shares the state directory.
	fmt.Fprintf(run.output, "sshc: stopping the engine (pid %d)\n", found.PID)
	if err := stopRunningEngine(ctx, paths.stateDir, found, client, lockEngineStart); err != nil {
		return err
	}
	if run.startService != nil {
		fmt.Fprintln(run.output, "sshc: starting the sshc service")
		started, err := run.startService(ctx, job.Plan.Installation.Executable)
		if err != nil || started {
			return err
		}
	}
	fmt.Fprintf(run.output, "sshc: starting the engine on port %s\n", engineURL.Port())
	environment := replaceEnvironment(os.Environ(), map[string]string{webUpdateRestartEnvironment: ""})
	// Keep this browser's origin even when --port overrode the saved engine settings.
	return run.startProcess(job.Plan.Installation.Executable, []string{"engine", "--background", "--port", engineURL.Port()}, environment)
}
