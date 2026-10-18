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

func TestWebUpdateUsesTheInstallationManagerAndKeepsTheInstalledVersion(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	for _, scenario := range []struct {
		name    string
		manager installManager
	}{{name: "Homebrew", manager: managerHomebrew}, {name: "install.sh", manager: managerShell}, {name: "install.ps1", manager: managerPowerShell}} {
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
				if found.manager != manager || release.Version != "v1.1.0" {
					t.Errorf("unexpected installation = %+v, release=%+v", found, release)
				}
				return nil
			}
			install := installationDependencies.install
			installationDependencies.installWeb = func(ctx context.Context, found installation, release releasecheck.Release) error {
				return install(ctx, found, release, io.Discard, io.Discard)
			}
			if manager == managerPowerShell {
				installationDependencies.install = func(context.Context, installation, releasecheck.Release, io.Writer, io.Writer) error {
					t.Fatal("web update used the CLI installer that stops its caller")
					return nil
				}
			}
			installationDependencies.upgradeHomebrew = func(_ context.Context, found installation) (string, error) {
				installs.Add(1)
				if found.manager != managerHomebrew {
					t.Errorf("unexpected Homebrew installation = %+v", found)
				}
				return "v1.2.0", nil
			}
			dependencies := webUpdateDependencies(selfupdate.Dependencies{
				Current: "v1.0.0", PID: 101, StatePath: filepath.Join(t.TempDir(), selfupdate.StateFileName),
			}, installationDependencies)
			restarted := make(chan selfupdate.Job, 1)
			dependencies.Restart = func(_ context.Context, job selfupdate.Job) error { restarted <- job; return nil }
			service := selfupdate.New(dependencies)
			t.Cleanup(func() { _ = service.Stop() })
			plan, err := service.Prepare(context.Background(), "v1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			job, err := service.Start(plan)
			if err != nil {
				t.Fatal(err)
			}
			service.ResponseSent(job.ID, nil)
			select {
			case installed := <-restarted:
				want := "v1.1.0"
				if manager == managerHomebrew {
					want = "v1.2.0"
				}
				if installed.InstalledVersion != want || installed.Target != plan.Target {
					t.Fatalf("installation result = %+v", installed)
				}
			case <-time.After(webUpdateCompletionTimeout):
				t.Fatal("installation did not reach restart")
			}
			if installs.Load() != 1 {
				t.Fatalf("installer calls = %d", installs.Load())
			}
		})
	}
}

// Bound asynchronous worker checks without depending on installer duration.
const webUpdateCompletionTimeout = 3 * time.Second

func TestWebUpdateRefusesAChangedManagerImmediatelyBeforeInstallation(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	dependencies := webUpdateInstallationFixture(t)
	executable, _ := dependencies.executable()
	inspection, err := inspectWebInstallation(context.Background(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	dependencies.detect = func(string) (installation, error) {
		return installation{manager: managerHomebrew, executable: executable}, nil
	}
	dependencies.upgradeHomebrew = func(context.Context, installation) (string, error) {
		t.Error("installer ran after the installation manager changed")
		return "v1.1.0", nil
	}
	_, err = installWebUpdate(context.Background(), selfupdate.Plan{Target: "v1.1.0", Installation: inspection}, dependencies)
	if err != selfupdate.ErrChanged {
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
	for _, scenario := range []string{"managed", "foreground", "changed engine", "service failure", "Windows task", "Windows background", "Windows task failure", "Windows changed engine"} {
		t.Run(scenario, func(t *testing.T) {
			var stopped atomic.Int32
			client := backgroundTestClient(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.URL.Path != httpserver.StopPath || request.Header.Get(handoff.HeaderName) != testHandoff("").Secret {
					t.Error("unexpected engine request")
					response.WriteHeader(http.StatusForbidden)
					return
				}
				stopped.Add(1)
				response.WriteHeader(http.StatusAccepted)
			}))
			engineURL, err := url.Parse(backgroundTestURL)
			if err != nil {
				t.Fatal(err)
			}
			stateDirectory := t.TempDir()
			writeTestHandoff(t, stateDirectory, backgroundTestURL)
			job := selfupdate.Job{OwnerPID: testHandoff("").PID, Plan: selfupdate.Plan{Installation: selfupdate.Installation{Executable: "/fixture/updated/sshc"}}}
			if strings.Contains(scenario, "changed engine") {
				job.OwnerPID++
			}
			starts, serviceCalls := 0, 0
			run := webUpdateRestartRun{client: client, paths: userPaths{stateDir: stateDirectory}, job: job,
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
					if executable != job.Plan.Installation.Executable || !reflect.DeepEqual(arguments, []string{"engine", "--background", "--port", engineURL.Port()}) {
						t.Errorf("unexpected argv = %s %v", executable, arguments)
					}
					for _, entry := range environment {
						if strings.HasPrefix(entry, webUpdateRestartEnvironment+"=") && entry != webUpdateRestartEnvironment+"=" {
							t.Error("engine inherited helper mode")
						}
					}
					return nil
				}}
			if strings.HasPrefix(scenario, "Windows") {
				run.startService = func(_ context.Context, executable string) (bool, error) {
					serviceCalls++
					if stopped.Load() != 1 || executable != job.Plan.Installation.Executable {
						t.Fatal("task started before stopping the confirmed engine")
					}
					if scenario == "Windows task failure" {
						return false, errors.New("task failed")
					}
					return scenario == "Windows task", nil
				}
				run.restartService = func(context.Context, string) (bool, error) {
					t.Fatal("Windows tried to verify the renamed old image against the new task executable")
					return false, nil
				}
			}
			err = restartWebUpdateEngine(context.Background(), run)
			switch scenario {
			case "managed":
				if err != nil || starts != 0 || stopped.Load() != 0 || serviceCalls != 1 {
					t.Fatalf("managed restart err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			case "foreground", "Windows background":
				if err != nil || starts != 1 || stopped.Load() != 1 {
					t.Fatalf("foreground err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			case "Windows task":
				if err != nil || starts != 0 || stopped.Load() != 1 || serviceCalls != 1 {
					t.Fatalf("task restart err=%v starts=%d stops=%d service=%d", err, starts, stopped.Load(), serviceCalls)
				}
			case "Windows task failure":
				if err == nil || starts != 0 || stopped.Load() != 1 {
					t.Fatalf("failed task err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			default:
				if err == nil || starts != 0 || stopped.Load() != 0 {
					t.Fatalf("unsafe restart err=%v starts=%d stops=%d", err, starts, stopped.Load())
				}
			}
		})
	}
}
