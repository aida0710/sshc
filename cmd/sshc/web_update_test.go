package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
)

func webUpdateInstallationFixture(t *testing.T) updateDependencies {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "sshc")
	if err := os.WriteFile(executable, []byte("fixture executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	return updateDependencies{
		executable: func() (string, error) { return executable, nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell, executable: executable}, nil
		},
		latest: func(context.Context) (releasecheck.Release, error) {
			return releasecheck.Release{Version: "v1.1.0"}, nil
		},
		serviceExecutable: func(context.Context, installation) (string, error) { return executable, nil },
	}
}

func TestWebUpdateRefusesHomebrewBeforeInstallationAndKeepsTheShellInstaller(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	for _, scenario := range []struct {
		name    string
		manager installManager
	}{{name: "Homebrew", manager: managerHomebrew}, {name: "install.sh", manager: managerShell}} {
		t.Run(scenario.name, func(t *testing.T) {
			manager := scenario.manager
			installationDependencies := webUpdateInstallationFixture(t)
			executable, _ := installationDependencies.executable()
			installationDependencies.detect = func(string) (installation, error) {
				return installation{manager: manager, executable: executable}, nil
			}
			var installs atomic.Int32
			installationDependencies.install = func(_ context.Context, found installation, release releasecheck.Release, _, _ io.Writer) error {
				installs.Add(1)
				if found.manager != managerShell || release.Version != "v1.1.0" {
					t.Errorf("unexpected installation = %+v, release=%+v", found, release)
				}
				return nil
			}
			dependencies := webUpdateDependencies(selfupdate.Dependencies{
				Current: "v1.0.0", PID: 101, StatePath: filepath.Join(t.TempDir(), selfupdate.StateFileName),
			}, installationDependencies)
			restarted := make(chan struct{}, 1)
			dependencies.Restart = func(context.Context, selfupdate.Job) error { restarted <- struct{}{}; return nil }
			service := selfupdate.New(dependencies)
			t.Cleanup(func() { _ = service.Stop() })
			plan, err := service.Prepare(context.Background(), "v1.1.0")
			if manager == managerHomebrew {
				if err != selfupdate.Failure("update_homebrew_unsupported") {
					t.Fatalf("Homebrew preparation = %v", err)
				}
				// Even a plan reserved before this restriction must fail on reinspection.
				plan = selfupdate.Plan{Current: "v1.0.0", Target: "v1.1.0", Installation: selfupdate.Installation{Manager: "homebrew", Executable: executable}}
			} else if err != nil {
				t.Fatal(err)
			}
			job, err := service.Start(plan)
			if err != nil {
				t.Fatal(err)
			}
			service.ResponseSent(job.ID, nil)
			if manager == managerShell {
				select {
				case <-restarted:
				case <-time.After(webUpdateCompletionTimeout):
					t.Fatal("shell installation did not reach restart")
				}
			} else {
				awaitWebUpdateFailure(t, service)
			}
			if err := service.Stop(); err != nil {
				t.Fatal(err)
			}
			if manager == managerHomebrew && installs.Load() != 0 || manager == managerShell && installs.Load() != 1 {
				t.Fatalf("installer calls = %d", installs.Load())
			}
		})
	}
}

// Bound asynchronous worker checks without depending on installer duration.
const webUpdateCompletionTimeout = 3 * time.Second

func awaitWebUpdateFailure(t *testing.T, service *selfupdate.Service) {
	t.Helper()
	deadline := time.NewTimer(webUpdateCompletionTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := service.Status()
		if err != nil {
			t.Fatal(err)
		}
		if job.State == selfupdate.JobFailed {
			if job.Problem != "update_homebrew_unsupported" {
				t.Fatalf("refused job = %+v", job)
			}
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("Homebrew worker did not fail before installing")
		}
	}
}

func TestWebUpdateRefusesAChangeToHomebrewImmediatelyBeforeInstallation(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	dependencies := webUpdateInstallationFixture(t)
	executable, _ := dependencies.executable()
	inspection, err := inspectWebInstallation(context.Background(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	detections := 0
	dependencies.detect = func(string) (installation, error) {
		detections++
		manager := managerShell
		if detections > 1 {
			manager = managerHomebrew
		}
		return installation{manager: manager, executable: executable}, nil
	}
	dependencies.install = func(context.Context, installation, releasecheck.Release, io.Writer, io.Writer) error {
		t.Error("installer ran after manager changed to Homebrew")
		return nil
	}
	err = installWebUpdate(context.Background(), selfupdate.Plan{Target: "v1.1.0", Installation: inspection}, dependencies)
	if err != selfupdate.Failure("update_homebrew_unsupported") {
		t.Fatalf("changed manager = %v", err)
	}
}

func TestWebUpdateInspectionRequiresAMatchingReceiptAndWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	installationDirectory := t.TempDir()
	directory := filepath.Join(t.TempDir(), "installation")
	// A linked parent reproduces macOS temporary paths such as /var -> /private/var.
	if err := os.Symlink(installationDirectory, directory); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(directory, "sshc")
	contents := []byte("fixture executable")
	if err := os.WriteFile(executable, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	receipt, _ := json.Marshal(installReceipt{SchemaVersion: 1, Manager: "install.sh", Repository: installRepository, Version: "v1.0.0", SHA256: hex.EncodeToString(digest[:])})
	if err := os.WriteFile(filepath.Join(directory, receiptFileName), receipt, 0o644); err != nil {
		t.Fatal(err)
	}
	dependencies := defaultUpdateDependencies()
	dependencies.executable = func() (string, error) { return executable, nil }
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := inspectWebInstallation(context.Background(), dependencies)
	if err != nil || inspected.Manager != "install.sh" || inspected.Executable != resolvedExecutable {
		t.Fatalf("inspection = %+v, %v", inspected, err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 2 {
		t.Fatal("permission probe left a file behind")
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
	currentUser, userError := user.Current()
	if userError == nil && currentUser.Uid != "0" {
		_, err = inspectWebInstallation(context.Background(), dependencies)
		if !errors.Is(err, selfupdate.ErrPermission) {
			t.Fatalf("read-only directory = %v", err)
		}
	}
	_ = os.Chmod(directory, 0o700)
	if err := os.WriteFile(executable, []byte("manually replaced binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = inspectWebInstallation(context.Background(), dependencies)
	if err != selfupdate.Failure("update_unmanaged") {
		t.Fatalf("changed receipt = %v", err)
	}
}

func TestWebUpdateRestartUsesTheManagedServiceOrStopsOnlyTheConfirmedForegroundEngine(t *testing.T) {
	for _, scenario := range []string{"managed", "foreground", "changed engine", "service failure"} {
		t.Run(scenario, func(t *testing.T) {
			var stopped atomic.Int32
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != httpserver.StopPath || request.Header.Get(handoff.HeaderName) != testHandoff("").Secret {
					t.Error("unexpected engine request")
					response.WriteHeader(http.StatusForbidden)
					return
				}
				stopped.Add(1)
				response.WriteHeader(http.StatusAccepted)
			}))
			defer server.Close()
			engineURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			stateDirectory := t.TempDir()
			writeTestHandoff(t, stateDirectory, server.URL)
			job := selfupdate.Job{OwnerPID: testHandoff("").PID, Plan: selfupdate.Plan{Installation: selfupdate.Installation{Executable: "/fixture/updated/sshc"}}}
			if scenario == "changed engine" {
				job.OwnerPID++
			}
			starts, serviceCalls := 0, 0
			run := webUpdateRestartRun{paths: userPaths{stateDir: stateDirectory}, job: job,
				restartService: func(_ context.Context, executable string) (bool, error) {
					serviceCalls++
					if executable != job.Plan.Installation.Executable {
						t.Error("wrong managed executable")
					}
					if scenario == "service failure" {
						return false, errors.New("service restart failed")
					}
					return scenario == "managed", nil
				},
				startProcess: func(executable string, arguments, environment []string) error {
					starts++
					if executable != job.Plan.Installation.Executable || !reflect.DeepEqual(arguments, []string{"engine", "--port", engineURL.Port()}) {
						t.Errorf("unexpected argv = %s %v", executable, arguments)
					}
					for _, entry := range environment {
						if strings.HasPrefix(entry, webUpdateRestartEnvironment+"=") && entry != webUpdateRestartEnvironment+"=" {
							t.Error("engine inherited helper mode")
						}
					}
					return nil
				}}
			err = restartWebUpdateEngine(context.Background(), run)
			switch scenario {
			case "managed":
				if err != nil || starts != 0 || stopped.Load() != 0 || serviceCalls != 1 {
					t.Fatalf("managed restart err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			case "foreground":
				if err != nil || starts != 1 || stopped.Load() != 1 {
					t.Fatalf("foreground err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			default:
				if err == nil || starts != 0 || stopped.Load() != 0 {
					t.Fatalf("unsafe restart err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			}
		})
	}
}
