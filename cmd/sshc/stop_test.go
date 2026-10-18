package main

import (
	"context"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/httpserver"
)

func TestStopSucceedsWithoutAnEngineEvenWithAStaleHandoff(t *testing.T) {
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, backgroundTestURL)
	var stdout, stderr strings.Builder
	code := runStop(context.Background(), commandEnvironment{stateDir: stateDir, stdout: &stdout, stderr: &stderr})
	if code != 0 || stdout.String() != "sshc: no sshc engine is running\n" || stderr.Len() != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestStopAsksTheEngineToExitAndWaitsForItsLock(t *testing.T) {
	stateDir := t.TempDir()
	release := holdTestEngineLock(t, stateDir)
	stopped := make(chan struct{}, 1)
	client := backgroundTestClient(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case httpserver.StatusPath:
			io.WriteString(response, validEngineStatus())
		case httpserver.StopPath:
			if request.Method != http.MethodPost {
				t.Errorf("stop method = %s", request.Method)
			}
			response.WriteHeader(http.StatusAccepted)
			release()
			stopped <- struct{}{}
		default:
			t.Errorf("request = %s", request.URL.Path)
		}
	})
	writeTestHandoff(t, stateDir, backgroundTestURL)
	var stdout, stderr strings.Builder
	code := runStop(context.Background(), commandEnvironment{home: t.TempDir(), stateDir: stateDir, client: client, stdout: &stdout, stderr: &stderr})
	if code != 0 || len(stopped) != 1 || !strings.Contains(stdout.String(), "stopped the sshc engine") {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if running, err := isEngineRunning(stateDir); err != nil || running {
		t.Fatalf("lock is still held: %v, %v", running, err)
	}
}

func TestStopMentionsLoginOnlyForAnInstalledServiceOnSupportedPlatforms(t *testing.T) {
	for _, state := range []serviceState{serviceAbsent, serviceUnmanaged, serviceInactive, serviceActive} {
		var stderr strings.Builder
		reportServiceLoginReminder(context.Background(), commandEnvironment{stderr: &stderr}, func(string) (engineServiceManager, error) {
			return &fakeServiceManager{state: state}, nil
		})
		wantReminder := (runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "windows") && (state == serviceActive || state == serviceInactive)
		if strings.Contains(stderr.String(), "sshc service disable") != wantReminder {
			t.Fatalf("state = %v, stderr = %q", state, stderr.String())
		}
	}
}
