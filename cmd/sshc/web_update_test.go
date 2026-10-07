package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/selfupdate"
)

func TestWebUpdateInspectionRequiresAMatchingReceiptAndWritableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "android" {
		t.Skip("platform has no automatic installer")
	}
	directory := t.TempDir()
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
	inspected, err := inspectWebInstallation(context.Background(), dependencies)
	if err != nil || inspected.Manager != "install.sh" || inspected.Executable != executable {
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
					if executable != job.Plan.Installation.Executable || !reflect.DeepEqual(arguments, []string{"engine"}) {
						t.Errorf("unexpected argv = %s %v", executable, arguments)
					}
					for _, entry := range environment {
						if strings.HasPrefix(entry, webUpdateRestartEnvironment+"=") && entry != webUpdateRestartEnvironment+"=" {
							t.Error("engine inherited helper mode")
						}
					}
					return nil
				}}
			err := restartWebUpdateEngine(context.Background(), run)
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
