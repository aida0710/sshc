package selfupdate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// Fail a missing completion without leaving the suite blocked indefinitely.
	updateCompletionTimeout = 3 * time.Second
	// Give an incorrectly early Stop enough time to report completion.
	updatePendingWindow = 20 * time.Millisecond
)

func startUpdateFixture(t *testing.T, service *Service) Job {
	t.Helper()
	plan, err := service.Prepare(context.Background(), "v1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func awaitUpdateCompletion(t *testing.T, completed <-chan error) {
	t.Helper()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(updateCompletionTimeout):
		t.Fatal("update did not finish stopping")
	}
}

func TestStopCancelsInstallationAndWaitsForItsCompletion(t *testing.T) {
	dependencies := updateFixture(t)
	installationContext := make(chan context.Context, 1)
	allowInstallerExit := make(chan struct{})
	var releaseOnce sync.Once
	dependencies.Install = func(ctx context.Context, _ Plan) (string, error) {
		installationContext <- ctx
		<-ctx.Done()
		<-allowInstallerExit
		return "v1.1.0", ctx.Err()
	}
	dependencies.Restart = func(context.Context, Job) error {
		t.Error("interrupted installation restarted the engine")
		return nil
	}
	service := New(dependencies)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(allowInstallerExit) })
		_ = service.Stop()
	})
	job := startUpdateFixture(t, service)
	service.ResponseSent(job.ID, nil)
	var ctx context.Context
	select {
	case ctx = <-installationContext:
	case <-time.After(updateCompletionTimeout):
		t.Fatal("installer did not start")
	}

	service.BeginStopping()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("installer context = %v", ctx.Err())
	}
	if _, err := service.Start(job.Plan); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("reservation after stopping = %v", err)
	}
	service.ResponseSent(job.ID, nil)
	stopped := make(chan error, 2)
	for range cap(stopped) {
		go func() { stopped <- service.Stop() }()
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before the installer completed: %v", err)
	case <-time.After(updatePendingWindow):
	}
	releaseOnce.Do(func() { close(allowInstallerExit) })
	for range cap(stopped) {
		awaitUpdateCompletion(t, stopped)
	}
	interrupted, err := service.store.Read()
	if err != nil || interrupted.ID != job.ID || interrupted.State != JobFailed || interrupted.Problem != "update_interrupted" {
		t.Fatalf("durable interruption = %+v, %v", interrupted, err)
	}
	select {
	case <-installationContext:
		t.Fatal("a late response started another installer")
	default:
	}
}

func TestStoppingBeforeResponseSentPersistsInterruptionAndRejectsLateResponses(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		responseError error
	}{{name: "flushed"}, {name: "flush failed", responseError: errors.New("flush failed")}} {
		t.Run(scenario.name, func(t *testing.T) {
			dependencies := updateFixture(t)
			var installs atomic.Int32
			dependencies.Install = func(context.Context, Plan) (string, error) { installs.Add(1); return "v1.1.0", nil }
			service := New(dependencies)
			job := startUpdateFixture(t, service)
			service.BeginStopping()
			service.ResponseSent(job.ID, scenario.responseError)
			if _, err := service.Start(job.Plan); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("reservation after stopping = %v", err)
			}
			if err := service.Stop(); err != nil {
				t.Fatal(err)
			}
			service.ResponseSent(job.ID, scenario.responseError)
			if _, err := service.Start(job.Plan); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("reservation after Stop returned = %v", err)
			}
			interrupted, err := service.Status()
			if err != nil || interrupted.State != JobFailed || interrupted.Problem != "update_interrupted" || installs.Load() != 0 {
				t.Fatalf("late response = %+v, %v, installs=%d", interrupted, err, installs.Load())
			}
		})
	}
}

func TestParentCancellationRejectsReservationsAndResponseWorkers(t *testing.T) {
	dependencies := updateFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dependencies.Context = ctx
	dependencies.Install = func(context.Context, Plan) (string, error) {
		t.Error("installer ran after cancellation")
		return "v1.1.0", nil
	}
	service := New(dependencies)
	job := startUpdateFixture(t, service)
	cancel()
	if _, err := service.Start(job.Plan); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("reservation after cancellation = %v", err)
	}
	service.ResponseSent(job.ID, nil)
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
	interrupted, err := service.Status()
	if err != nil || interrupted.Problem != "update_interrupted" {
		t.Fatalf("canceled pending response = %+v, %v", interrupted, err)
	}
}

func TestConcurrentResponsesAndStopLeaveNoActiveJobOrInstaller(t *testing.T) {
	dependencies := updateFixture(t)
	var installs atomic.Int32
	dependencies.Install = func(ctx context.Context, _ Plan) (string, error) {
		installs.Add(1)
		<-ctx.Done()
		return "v1.1.0", ctx.Err()
	}
	service := New(dependencies)
	job := startUpdateFixture(t, service)
	start := make(chan struct{})
	var responses sync.WaitGroup
	const concurrentResponses = 12 // Exercise duplicate callbacks on both sides of the stopping gate.
	for range concurrentResponses {
		responses.Add(1)
		go func() {
			defer responses.Done()
			<-start
			service.ResponseSent(job.ID, nil)
		}()
	}
	stopped := make(chan error, 1)
	go func() { <-start; stopped <- service.Stop() }()
	close(start)
	awaitUpdateCompletion(t, stopped)
	responses.Wait()
	interrupted, err := service.Status()
	if err != nil || interrupted.State != JobFailed || interrupted.Problem != "update_interrupted" || installs.Load() > 1 {
		t.Fatalf("concurrent stopping = %+v, %v, installs=%d", interrupted, err, installs.Load())
	}
}

func TestStopWaitsForRestartLaunchWithoutHoldingTheStoppingGate(t *testing.T) {
	dependencies := updateFixture(t)
	launchStarted := make(chan struct{})
	allowLaunchReturn := make(chan struct{})
	var releaseOnce sync.Once
	var service *Service
	dependencies.Restart = func(context.Context, Job) error {
		// A launched helper can request shutdown before the launch returns.
		service.BeginStopping()
		close(launchStarted)
		<-allowLaunchReturn
		return nil
	}
	service = New(dependencies)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(allowLaunchReturn) })
		_ = service.Stop()
	})
	job := startUpdateFixture(t, service)
	service.ResponseSent(job.ID, nil)
	select {
	case <-launchStarted:
	case <-time.After(updateCompletionTimeout):
		t.Fatal("restart launch deadlocked while requesting shutdown")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- service.Stop() }()
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned before restart launch completed: %v", err)
	case <-time.After(updatePendingWindow):
	}
	releaseOnce.Do(func() { close(allowLaunchReturn) })
	awaitUpdateCompletion(t, stopped)
	job, err := service.Status()
	if err != nil || job.State != JobRestarting {
		t.Fatalf("launched restart = %+v, %v", job, err)
	}
	dependencies.Current = job.Target
	confirmed, err := New(dependencies).Status()
	if err != nil || confirmed.State != JobSucceeded {
		t.Fatalf("new engine confirmation = %+v, %v", confirmed, err)
	}
}

func TestCanceledRestartLaunchKeepsTheInstalledReleaseForManualRecovery(t *testing.T) {
	dependencies := updateFixture(t)
	launchStarted := make(chan struct{})
	dependencies.Restart = func(ctx context.Context, _ Job) error {
		close(launchStarted)
		<-ctx.Done()
		return ctx.Err()
	}
	service := New(dependencies)
	job := startUpdateFixture(t, service)
	service.ResponseSent(job.ID, nil)
	select {
	case <-launchStarted:
	case <-time.After(updateCompletionTimeout):
		t.Fatal("restart launch did not start")
	}
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
	installed, err := service.Status()
	if err != nil || installed.State != JobRestartRequired || installed.Problem != "update_restart_failed" {
		t.Fatalf("canceled restart = %+v, %v", installed, err)
	}
}
