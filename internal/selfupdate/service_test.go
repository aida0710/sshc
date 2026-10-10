package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/releasecheck"
)

func updateFixture(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{Current: "v1.0.0", PID: 101, StatePath: filepath.Join(t.TempDir(), StateFileName),
		Latest: func(context.Context) (releasecheck.Release, error) {
			return releasecheck.Release{Version: "v1.1.0"}, nil
		},
		Inspect: func(context.Context) (Installation, error) {
			return Installation{Manager: "install.sh", Executable: "/fixture/sshc", Identity: "receipt-digest"}, nil
		},
		Install: func(context.Context, Plan) (string, error) { return "v1.1.0", nil }, Restart: func(context.Context, Job) error { return nil }}
}

func waitForUpdateState(t *testing.T, service *Service, state string) Job {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := service.Status()
		if err != nil {
			t.Fatal(err)
		}
		if job.State == state {
			return job
		}
		select {
		case <-deadline.C:
			t.Fatalf("job = %+v, want %s", job, state)
		case <-ticker.C:
		}
	}
}

func TestUpdateWaitsForResponseAndOnlyOneInstallerRuns(t *testing.T) {
	dependencies := updateFixture(t)
	installed := make(chan struct{}, 1)
	allowInstall := make(chan struct{})
	dependencies.Install = func(context.Context, Plan) (string, error) {
		installed <- struct{}{}
		<-allowInstall
		return "v1.1.0", nil
	}
	service := New(dependencies)
	plan, err := service.Prepare(context.Background(), "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-installed:
		t.Fatal("installer ran before the response")
	default:
	}
	if _, err := service.Start(plan); !errors.Is(err, ErrBusy) {
		t.Fatalf("duplicate = %v", err)
	}
	service.ResponseSent(job.ID, nil)
	service.ResponseSent(job.ID, nil)
	service.ResponseSent(job.ID, errors.New("late duplicate response failed"))
	select {
	case <-installed:
	case <-time.After(time.Second):
		t.Fatal("installer did not start")
	}
	if _, err := service.Prepare(context.Background(), "v1.1.0"); !errors.Is(err, ErrBusy) {
		t.Fatalf("preview during installation = %v", err)
	}
	close(allowInstall)
	waitForUpdateState(t, service, "restarting")
	select {
	case <-installed:
		t.Fatal("installer ran twice")
	default:
	}
	dependencies.PID, dependencies.Current = 202, "v1.1.0"
	recovered := New(dependencies)
	confirmed, err := recovered.Status()
	if err != nil || confirmed.State != "succeeded" || confirmed.ID != job.ID {
		t.Fatalf("recovered = %+v, %v", confirmed, err)
	}
}

func TestFailedInstallationNeverRestartsAndRequiresANewReservation(t *testing.T) {
	dependencies := updateFixture(t)
	dependencies.Install = func(context.Context, Plan) (string, error) {
		return "v1.1.0", errors.New("installer failed with private output")
	}
	restarted := false
	dependencies.Restart = func(context.Context, Job) error { restarted = true; return nil }
	service := New(dependencies)
	plan, _ := service.Prepare(context.Background(), "v1.1.0")
	job, err := service.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	service.ResponseSent(job.ID, nil)
	failed := waitForUpdateState(t, service, "failed")
	if restarted || failed.Problem != "update_install_failed" {
		t.Fatalf("failure = %+v, restart=%v", failed, restarted)
	}
	retry, err := service.Start(plan)
	if err != nil || retry.ID == job.ID {
		t.Fatalf("retry = %+v, %v", retry, err)
	}
	service.ResponseSent(retry.ID, errors.New("response failed"))
}

func TestRestartFailureKeepsTheInstalledResultForManualRecovery(t *testing.T) {
	dependencies := updateFixture(t)
	dependencies.Restart = func(context.Context, Job) error { return errors.New("helper cannot start") }
	service := New(dependencies)
	plan, _ := service.Prepare(context.Background(), "v1.1.0")
	job, _ := service.Start(plan)
	service.ResponseSent(job.ID, nil)
	recovered := waitForUpdateState(t, service, "restart_required")
	if recovered.Problem != "update_restart_failed" {
		t.Fatalf("job = %+v", recovered)
	}
}

func TestFailedResponseAndChangedInstallationNeverRunAnInstaller(t *testing.T) {
	for _, scenario := range []string{"response", "installation"} {
		t.Run(scenario, func(t *testing.T) {
			dependencies := updateFixture(t)
			dependencies.Install = func(context.Context, Plan) (string, error) { t.Error("installer ran"); return "v1.1.0", nil }
			service := New(dependencies)
			plan, _ := service.Prepare(context.Background(), "v1.1.0")
			job, _ := service.Start(plan)
			var responseError error
			if scenario == "response" {
				responseError = errors.New("response was not sent")
			} else {
				service.dependencies.Inspect = func(context.Context) (Installation, error) {
					return Installation{Manager: "install.sh", Identity: "different-file"}, nil
				}
			}
			service.ResponseSent(job.ID, responseError)
			waitForUpdateState(t, service, "failed")
		})
	}
}

func TestInterruptedJobIsRecoveredWithoutRepeatingTheInstaller(t *testing.T) {
	for _, scenario := range []string{"different PID", "reused PID"} {
		t.Run(scenario, func(t *testing.T) {
			dependencies := updateFixture(t)
			service := New(dependencies)
			plan, _ := service.Prepare(context.Background(), "v1.1.0")
			job, _ := service.Start(plan)
			if scenario == "different PID" {
				dependencies.PID++
			}
			recovered, err := New(dependencies).Status()
			if err != nil || recovered.State != "failed" || recovered.Problem != "update_interrupted" || recovered.ID != job.ID {
				t.Fatalf("job = %+v, %v", recovered, err)
			}
		})
	}
}

func TestDevelopmentAndUnpublishedVersionsCannotBePrepared(t *testing.T) {
	dependencies := updateFixture(t)
	for _, target := range []string{"v1.0.0", "v2.0.0", "dev", "v1.1.0; touch /tmp/unwanted", "1.1.0"} {
		if _, err := New(dependencies).Prepare(context.Background(), target); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	dependencies.Current = "dev"
	if _, err := New(dependencies).Prepare(context.Background(), "v1.1.0"); err != Failure("update_development_build") {
		t.Fatalf("dev = %v", err)
	}
}

func TestManualRestartConfirmsTheInstalledReleaseWithoutReinstalling(t *testing.T) {
	dependencies := updateFixture(t)
	dependencies.Restart = func(context.Context, Job) error { return errors.New("restart could not start") }
	service := New(dependencies)
	plan, _ := service.Prepare(context.Background(), "v1.1.0")
	job, _ := service.Start(plan)
	service.ResponseSent(job.ID, nil)
	waitForUpdateState(t, service, "restart_required")
	if _, err := service.Prepare(context.Background(), "v1.1.0"); err != Failure("update_restart_failed") {
		t.Fatalf("another update = %v", err)
	}
	dependencies.PID = 202
	olderEngine := New(dependencies)
	if pending, err := olderEngine.Status(); err != nil || pending.State != JobRestartRequired {
		t.Fatalf("older engine lost manual recovery = %+v, %v", pending, err)
	}
	if _, err := olderEngine.Start(plan); err != Failure("update_restart_failed") {
		t.Fatalf("reinstalled without confirming restart = %v", err)
	}
	dependencies.PID, dependencies.Current = 202, "v1.1.0"
	confirmed, err := New(dependencies).Status()
	if err != nil || confirmed.State != "succeeded" {
		t.Fatalf("manual restart = %+v, %v", confirmed, err)
	}
}

func TestConcurrentStartsReserveExactlyOneDurableJob(t *testing.T) {
	dependencies := updateFixture(t)
	service := New(dependencies)
	plan, _ := service.Prepare(context.Background(), "v1.1.0")
	attempts := make(chan error, 12)
	for range cap(attempts) {
		go func() { _, err := service.Start(plan); attempts <- err }()
	}
	reserved := 0
	for range cap(attempts) {
		err := <-attempts
		if err == nil {
			reserved++
		} else if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
	}
	if reserved != 1 {
		t.Fatalf("reserved %d jobs", reserved)
	}
}

func TestStateSaveFailureAfterInstallationNeverRestarts(t *testing.T) {
	dependencies := updateFixture(t)
	restarted := false
	dependencies.Install = func(context.Context, Plan) (string, error) {
		if err := os.Remove(dependencies.StatePath); err != nil {
			return "v1.1.0", err
		}
		return "v1.1.0", os.Mkdir(dependencies.StatePath, 0o700)
	}
	dependencies.Restart = func(context.Context, Job) error { restarted = true; return nil }
	service := New(dependencies)
	plan, _ := service.Prepare(context.Background(), "v1.1.0")
	job, _ := service.Start(plan)
	service.ResponseSent(job.ID, nil)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if _, err := service.Status(); errors.Is(err, ErrState) {
			if restarted {
				t.Fatal("restarted without saving the installation result")
			}
			return
		}
	}
	t.Fatal("state save failure was hidden")
}
