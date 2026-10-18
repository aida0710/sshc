package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/enginelog"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// Lifecycle decisions need HTTP semantics, but no listening socket. The same
// challenge helper is used by the existing tests with real HTTP servers.
const backgroundTestURL = "http://127.0.0.1:54447"

func backgroundTestClient(handler http.HandlerFunc) *http.Client {
	return &http.Client{Transport: testHTTPTransport(func(request *http.Request) (*http.Response, error) {
		if proof := proofResponse(request); proof != nil {
			return proof, nil
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Result(), nil
	})}
}

func holdTestEngineLock(t *testing.T, stateDir string) func() error {
	t.Helper()
	release, err := lockEngineStart(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = release() })
	return release
}

func TestAnAvailableLockStartsAnEngineWithALockedVaultEvenWithAStaleHandoff(t *testing.T) {
	client := backgroundTestClient(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != httpserver.StatusPath {
			t.Errorf("unexpected request %s", request.URL.Path)
		}
		io.WriteString(response, strings.Replace(validEngineStatus(), `"unlocked":true`, `"unlocked":false`, 1))
	})
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, backgroundTestURL)
	var stderr strings.Builder
	started, released := false, false
	found, err := ensureBackgroundEngine(context.Background(), commandEnvironment{stateDir: stateDir, client: client, stderr: &stderr}, func(string) (*detachedProcess, error) {
		started = true
		holdTestEngineLock(t, stateDir)
		writeTestHandoff(t, stateDir, backgroundTestURL)
		return &detachedProcess{pid: testHandoff(backgroundTestURL).PID, release: func() { released = true }}, nil
	})
	if err != nil || found.URL != backgroundTestURL || !started || !released || !strings.Contains(stderr.String(), enginelog.Path(stateDir)) {
		t.Fatalf("found = %+v, err = %v, started = %v, released = %v, stderr = %q", found, err, started, released, stderr.String())
	}
}

func TestAHeldLockWaitsForTheEngineToPublishItsHandoff(t *testing.T) {
	stateDir := t.TempDir()
	holdTestEngineLock(t, stateDir)
	readyClient := backgroundTestClient(func(response http.ResponseWriter, request *http.Request) {
		io.WriteString(response, validEngineStatus())
	})
	// The first proof attempt publishes the handoff only after open has begun.
	requested := false
	client := &http.Client{Transport: testHTTPTransport(func(request *http.Request) (*http.Response, error) {
		if !requested {
			requested = true
			writeTestHandoff(t, stateDir, backgroundTestURL)
			return nil, errors.New("handoff not ready")
		}
		return readyClient.Transport.RoundTrip(request)
	})}
	writeTestHandoff(t, stateDir, backgroundTestURL)
	found, err := ensureBackgroundEngine(context.Background(), commandEnvironment{stateDir: stateDir, client: client, stderr: io.Discard}, func(string) (*detachedProcess, error) {
		t.Fatal("started a second engine while the lock was held")
		return nil, nil
	})
	if err != nil || found.URL != backgroundTestURL || !requested {
		t.Fatalf("found = %+v, err = %v", found, err)
	}
}

func TestAnEngineThatExitsBeforeReadinessReportsItsCodeAndLogTail(t *testing.T) {
	stateDir := t.TempDir()
	file, err := enginelog.Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	for line := 0; line < enginelog.TailLines+1; line++ {
		fmt.Fprintf(file, "startup line %d\n", line)
	}
	file.Close()
	exited := make(chan processExit, 1)
	exited <- processExit{code: 7}
	_, err = ensureBackgroundEngine(context.Background(), commandEnvironment{stateDir: stateDir, stderr: io.Discard}, func(string) (*detachedProcess, error) {
		return &detachedProcess{pid: 123, exited: exited, release: func() {}}, nil
	})
	var failure *engineExitedError
	if !errors.As(err, &failure) || !strings.Contains(err.Error(), "exit code 7") || strings.Contains(err.Error(), "startup line 0\n") || !strings.Contains(err.Error(), fmt.Sprintf("startup line %d", enginelog.TailLines)) {
		t.Fatalf("failure = %v", err)
	}
}

func TestAChildThatLosesTheLockUsesTheWinningEnginesHandoff(t *testing.T) {
	client := backgroundTestClient(func(response http.ResponseWriter, request *http.Request) {
		io.WriteString(response, validEngineStatus())
	})
	stateDir := t.TempDir()
	exited := make(chan processExit, 1)
	exited <- processExit{code: 1}
	found, err := ensureBackgroundEngine(context.Background(), commandEnvironment{stateDir: stateDir, client: client, stderr: io.Discard}, func(string) (*detachedProcess, error) {
		holdTestEngineLock(t, stateDir)
		writeTestHandoff(t, stateDir, backgroundTestURL)
		return &detachedProcess{pid: testHandoff(backgroundTestURL).PID + 1, exited: exited, release: func() {}}, nil
	})
	if err != nil || found.URL != backgroundTestURL {
		t.Fatalf("found = %+v, err = %v", found, err)
	}
}

func TestAnExitedChildIsReportedIfTheWinningEngineNeverBecomesReady(t *testing.T) {
	stateDir := t.TempDir()
	holdTestEngineLock(t, stateDir)
	exited := make(chan processExit, 1)
	exited <- processExit{code: 1}
	_, err := waitForReadyEngine(context.Background(), stateDir, engineReadiness{
		timeout: engineReadyPollInterval,
		process: &detachedProcess{exited: exited},
	})
	var failure *engineExitedError
	if !errors.As(err, &failure) {
		t.Fatalf("lost child exit code: %v", err)
	}
	var stderr strings.Builder
	code := reportEngineUnreachable(context.Background(), errors.Join(err, context.DeadlineExceeded), &stderr)
	if code != exitFailure || !strings.Contains(stderr.String(), "exit code 1") {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
}

func TestReadinessHonorsCancellationWhileTheLockHasNoHandoff(t *testing.T) {
	stateDir := t.TempDir()
	holdTestEngineLock(t, stateDir)
	ctx, cancel := context.WithTimeout(context.Background(), engineReadyPollInterval)
	defer cancel()
	started := time.Now()
	_, err := ensureBackgroundEngine(ctx, commandEnvironment{stateDir: stateDir, stderr: io.Discard}, func(string) (*detachedProcess, error) {
		t.Fatal("unexpected start")
		return nil, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) >= backgroundReadyTimeout {
		t.Fatalf("readiness = %v", err)
	}
}

func TestOpenCancellationReturns130WithoutAdvice(t *testing.T) {
	for _, phase := range []string{httpserver.ChallengePath, httpserver.OpenPath} {
		t.Run(phase, func(t *testing.T) {
			stateDir := t.TempDir()
			holdTestEngineLock(t, stateDir)
			writeTestHandoff(t, stateDir, backgroundTestURL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &http.Client{Transport: testHTTPTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != phase {
					return proofResponse(request), nil
				}
				cancel()
				return nil, request.Context().Err()
			})}
			var stdout, stderr strings.Builder
			code := runOpen(ctx, commandEnvironment{stateDir: stateDir, client: client, stdout: &stdout, stderr: &stderr}, false)
			if code != exitInterrupted || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestAnUnprovenHandoffReportsItsNativePath(t *testing.T) {
	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, backgroundTestURL)
	client := &http.Client{Transport: testHTTPTransport(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody}, nil
	})}
	_, err := verifiedHandoff(context.Background(), stateDir, client)
	if !errors.Is(err, errEngineUnproven) || !strings.Contains(err.Error(), filepath.Join(stateDir, handoff.FileName)) || strings.Contains(err.Error(), "~/.ssh") {
		t.Fatalf("unproven handoff advice = %v", err)
	}
}

func TestOpenUsesTheRunningEngineAndPrintsItsOneTimeURL(t *testing.T) {
	stateDir := t.TempDir()
	holdTestEngineLock(t, stateDir)
	writeTestHandoff(t, stateDir, backgroundTestURL)
	client := backgroundTestClient(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != httpserver.OpenPath || request.Method != http.MethodPost {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		fmt.Fprintf(response, `{"url":%q}`, backgroundTestURL+"/#bootstrap=once")
	})
	var stdout, stderr strings.Builder
	code := runOpen(context.Background(), commandEnvironment{stateDir: stateDir, client: client, stdout: &stdout, stderr: &stderr}, false)
	if code != 0 || stdout.String() != backgroundTestURL+"/#bootstrap=once\n" || stderr.Len() != 0 {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}
