package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"sshc/internal/filelock"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
)

const (
	// Catch lost shutdown completions without hanging the test process.
	updateShutdownTestTimeout = 3 * time.Second
	// Observe early completion while the installer is deliberately blocked.
	updateShutdownPendingWindow = 20 * time.Millisecond
	// The in-memory listener only needs a valid browser origin; it binds no OS port.
	updateShutdownTestPort = 54321
)

// updateShutdownListener runs the real HTTP server over memory connections, so
// engine shutdown tests require no sockets or connection to a running engine.
type updateShutdownListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newUpdateShutdownListener() *updateShutdownListener {
	return &updateShutdownListener{connections: make(chan net.Conn), closed: make(chan struct{})}
}

func (listener *updateShutdownListener) Accept() (net.Conn, error) {
	select {
	case connection := <-listener.connections:
		return connection, nil
	case <-listener.closed:
		return nil, net.ErrClosed
	}
}

func (listener *updateShutdownListener) Close() error {
	listener.closeOnce.Do(func() { close(listener.closed) })
	return nil
}

func (listener *updateShutdownListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: updateShutdownTestPort}
}

func (listener *updateShutdownListener) connect(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case listener.connections <- server:
		return client, nil
	case <-listener.closed:
		_ = client.Close()
		_ = server.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, ctx.Err()
	}
}

func shutdownUpdateFixture(t *testing.T, stateDirectory string) selfupdate.Dependencies {
	t.Helper()
	return selfupdate.Dependencies{
		Current: "v1.0.0", PID: 4242, StatePath: filepath.Join(stateDirectory, selfupdate.StateFileName),
		Latest: func(context.Context) (releasecheck.Release, error) {
			return releasecheck.Release{Version: "v1.1.0"}, nil
		},
		Inspect: func(context.Context) (selfupdate.Installation, error) {
			return selfupdate.Installation{Manager: "install.sh", Executable: "/fixture/sshc", Identity: "receipt-digest"}, nil
		},
		Restart: func(context.Context, selfupdate.Job) error { t.Error("interrupted installation restarted"); return nil },
	}
}

func requestUpdateEngineStop(t *testing.T, listener *updateShutdownListener, stateDirectory string) {
	t.Helper()
	document, err := handoff.Read(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{DialContext: listener.connect}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: updateShutdownTestTimeout}
	request, err := http.NewRequest(http.MethodPost, document.URL+httpserver.StopPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(handoff.HeaderName, document.Secret)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("stop request = %d", response.StatusCode)
	}
}

func TestRunCancelsSelfUpdateAndKeepsTheEngineLockUntilInstallerCompletion(t *testing.T) {
	for _, stopping := range []string{"HTTP stop", "context cancellation"} {
		t.Run(stopping, func(t *testing.T) {
			dependencies := testDependencies(t)
			listener := newUpdateShutdownListener()
			dependencies.Listen = func(string, string) (net.Listener, error) { return listener, nil }
			stateDirectory := stateDirOf(t, dependencies.Home)
			updateDependencies := shutdownUpdateFixture(t, stateDirectory)
			installationContext := make(chan context.Context, 1)
			allowInstallerExit := make(chan struct{})
			var releaseOnce sync.Once
			updateDependencies.Install = func(ctx context.Context, _ selfupdate.Plan) (string, error) {
				installationContext <- ctx
				<-ctx.Done()
				<-allowInstallerExit
				return "", ctx.Err()
			}
			dependencies.SelfUpdate = selfupdate.New(updateDependencies)
			ready := make(chan struct{}, 1)
			dependencies.Announce = func(Readiness) error { ready <- struct{}{}; return nil }
			ctx, cancel := context.WithCancel(context.Background())
			lockPath := filepath.Join(stateDirectory, "engine.lock")
			releaseLock, err := filelock.TryAcquire(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			runCompleted := make(chan error, 1)
			runFinished := make(chan struct{})
			go func() {
				defer close(runFinished)
				// Match the desktop composition root: release only after Run returns.
				runError := Run(ctx, dependencies, "v1.0.0")
				runCompleted <- errors.Join(runError, releaseLock())
			}()
			t.Cleanup(func() {
				cancel()
				releaseOnce.Do(func() { close(allowInstallerExit) })
				_ = dependencies.SelfUpdate.Stop()
				select {
				case <-runFinished:
				case <-time.After(updateShutdownTestTimeout):
					t.Error("Run did not finish during test cleanup")
				}
			})
			select {
			case <-ready:
			case err := <-runCompleted:
				t.Fatalf("Run failed to start: %v", err)
			case <-time.After(updateShutdownTestTimeout):
				t.Fatal("Run did not announce readiness")
			}
			plan, err := dependencies.SelfUpdate.Prepare(context.Background(), "v1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			job, err := dependencies.SelfUpdate.Start(plan)
			if err != nil {
				t.Fatal(err)
			}
			dependencies.SelfUpdate.ResponseSent(job.ID, nil)
			var installerContext context.Context
			select {
			case installerContext = <-installationContext:
			case <-time.After(updateShutdownTestTimeout):
				t.Fatal("installer did not start")
			}
			if stopping == "HTTP stop" {
				requestUpdateEngineStop(t, listener, stateDirectory)
			} else {
				cancel()
			}
			select {
			case <-installerContext.Done():
			case <-time.After(updateShutdownTestTimeout):
				t.Fatal("engine stopping did not cancel the installer")
			}
			if _, err := dependencies.SelfUpdate.Start(plan); !errors.Is(err, selfupdate.ErrUnavailable) {
				t.Fatalf("reservation during engine shutdown = %v", err)
			}
			dependencies.SelfUpdate.ResponseSent(job.ID, nil)
			select {
			case err := <-runCompleted:
				t.Fatalf("Run returned before installer completion: %v", err)
			case <-time.After(updateShutdownPendingWindow):
			}
			if release, err := filelock.TryAcquire(lockPath); !errors.Is(err, filelock.ErrHeld) {
				if release != nil {
					_ = release()
				}
				t.Fatalf("engine lock while installer is blocked = %v", err)
			}
			releaseOnce.Do(func() { close(allowInstallerExit) })
			select {
			case err := <-runCompleted:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(updateShutdownTestTimeout):
				t.Fatal("engine did not finish after installer completion")
			}
			interrupted, err := dependencies.SelfUpdate.Status()
			if err != nil || interrupted.State != selfupdate.JobFailed || interrupted.Problem != "update_interrupted" {
				t.Fatalf("stopped engine update = %+v, %v", interrupted, err)
			}
			release, err := filelock.TryAcquire(lockPath)
			if err != nil {
				t.Fatalf("engine lock was not released after installer completion: %v", err)
			}
			_ = release()
		})
	}
}

func TestRunPersistsAnAcceptedUpdateAsInterruptedWhenStoppingBeforeItsResponse(t *testing.T) {
	dependencies := testDependencies(t)
	listener := newUpdateShutdownListener()
	dependencies.Listen = func(string, string) (net.Listener, error) { return listener, nil }
	updateDependencies := shutdownUpdateFixture(t, stateDirOf(t, dependencies.Home))
	updateDependencies.Install = func(context.Context, selfupdate.Plan) (string, error) {
		t.Error("pending response started installation")
		return "", nil
	}
	dependencies.SelfUpdate = selfupdate.New(updateDependencies)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var accepted selfupdate.Job
	dependencies.Announce = func(Readiness) error {
		plan, err := dependencies.SelfUpdate.Prepare(context.Background(), "v1.1.0")
		if err != nil {
			return err
		}
		accepted, err = dependencies.SelfUpdate.Start(plan)
		cancel()
		return err
	}
	if err := Run(ctx, dependencies, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	dependencies.SelfUpdate.ResponseSent(accepted.ID, nil)
	if _, err := dependencies.SelfUpdate.Start(accepted.Plan); !errors.Is(err, selfupdate.ErrUnavailable) {
		t.Fatalf("reservation after Run returned = %v", err)
	}
	interrupted, err := dependencies.SelfUpdate.Status()
	if err != nil || interrupted.State != selfupdate.JobFailed || interrupted.Problem != "update_interrupted" {
		t.Fatalf("pending response after Run returned = %+v, %v", interrupted, err)
	}
}
