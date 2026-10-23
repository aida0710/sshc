package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/releasecheck"
	"sshc/internal/releasewatch"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
)

type webUpdateHarness struct {
	engine           *echo.Echo
	handlers         *UpdateHandlers
	credentials      session.Credentials
	updater          *selfupdate.Service
	installation     selfupdate.Installation
	dependencies     selfupdate.Dependencies
	installed        atomic.Int32
	installedVersion string
	restarted        atomic.Int32
	installResult    chan error
	// installerOutput is what the fake installer writes before it waits for installResult.
	installerOutput string
	locked          bool
	sessions        *session.Manager
}

func newWebUpdateHarness(t *testing.T) *webUpdateHarness {
	t.Helper()
	harness := &webUpdateHarness{installedVersion: "v1.1.0", engine: echo.New(), installation: selfupdate.Installation{Manager: "install.sh", Executable: "/fixture/sshc", Identity: "receipt-digest"}, installResult: make(chan error, 1)}
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	harness.sessions = manager
	harness.credentials, _, err = manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	harness.dependencies = selfupdate.Dependencies{Current: "v1.0.0", PID: 101, StatePath: filepath.Join(t.TempDir(), selfupdate.StateFileName),
		Latest: func(context.Context) (releasecheck.Release, error) {
			return releasecheck.Release{Version: "v1.1.0"}, nil
		},
		Inspect: func(context.Context) (selfupdate.Installation, error) { return harness.installation, nil },
		Install: func(ctx context.Context, request selfupdate.InstallRequest) (string, error) {
			harness.installed.Add(1)
			_, _ = request.Output.Write([]byte(harness.installerOutput))
			select {
			case err := <-harness.installResult:
				return harness.installedVersion, err
			case <-ctx.Done():
				return "v1.1.0", ctx.Err()
			}
		},
		Restart: func(context.Context, selfupdate.Job) error { harness.restarted.Add(1); return nil }}
	harness.updater = selfupdate.New(harness.dependencies)
	harness.engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: func() bool { return !harness.locked }}).Middleware)
	harness.handlers = &UpdateHandlers{Current: "v1.0.0", Service: harness.updater, Actions: ActionHandlers{Sessions: manager}}
	registerUpdateRoutes(harness.engine, harness.handlers)
	return harness
}

func (harness *webUpdateHarness) preview(t *testing.T) api.UpdatePreview {
	t.Helper()
	response := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update/preview", []byte(`{"target":"v1.1.0"}`), "")
	if response.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", response.Code, response.Body.String())
	}
	var preview api.UpdatePreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

func awaitWebUpdate(t *testing.T, harness *webUpdateHarness, state string) api.UpdateStatus {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		response := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodGet, "/api/v1/update", nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var status api.UpdateStatus
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Job != nil && string(status.Job.State) == state {
			return status
		}
	}
	t.Fatalf("did not reach %s", state)
	return api.UpdateStatus{}
}

func TestWebUpdateStartsOnceReportsFailureAndRejectsTokenReplay(t *testing.T) {
	harness := newWebUpdateHarness(t)
	preview := harness.preview(t)
	if preview.ActionToken == "" || harness.installed.Load() != 0 {
		t.Fatal("preview started installation or lacked confirmation")
	}
	body := []byte(`{"target":"v1.1.0"}`)
	accepted := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", body, preview.ActionToken)
	if accepted.Code != http.StatusAccepted || !accepted.Flushed {
		t.Fatalf("acceptance = %d flushed=%v: %s", accepted.Code, accepted.Flushed, accepted.Body.String())
	}
	awaitWebUpdate(t, harness, "installing")
	duplicate := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", body, preview.ActionToken)
	if duplicate.Code != http.StatusConflict || problemCode(t, duplicate.Body.Bytes()) != "update_in_progress" {
		t.Fatalf("duplicate = %d %s", duplicate.Code, duplicate.Body.String())
	}
	harness.installResult <- errors.New("private installer output must not appear")
	failed := awaitWebUpdate(t, harness, "failed")
	if failed.Job.Problem != "update_install_failed" || harness.restarted.Load() != 0 {
		t.Fatalf("failure = %+v", failed)
	}
	replay := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", body, preview.ActionToken)
	if replay.Code != http.StatusForbidden || problemCode(t, replay.Body.Bytes()) != "action_token_invalid" {
		t.Fatalf("replay = %d %s", replay.Code, replay.Body.String())
	}
	if harness.installed.Load() != 1 {
		t.Fatalf("installer count = %d", harness.installed.Load())
	}
}

func TestWebUpdateResultSurvivesRestartAndIsReadableWithALockedVault(t *testing.T) {
	for _, scenario := range []struct{ manager, installedVersion string }{
		{manager: "install.sh", installedVersion: "v1.1.0"},
		{manager: "homebrew", installedVersion: "v1.2.0"},
	} {
		t.Run(scenario.manager, func(t *testing.T) {
			harness := newWebUpdateHarness(t)
			harness.installation.Manager = scenario.manager
			harness.installedVersion = scenario.installedVersion
			preview := harness.preview(t)
			accepted := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", []byte(`{"target":"v1.1.0"}`), preview.ActionToken)
			if accepted.Code != http.StatusAccepted {
				t.Fatal(accepted.Body.String())
			}
			harness.installResult <- nil
			awaitWebUpdate(t, harness, "restarting")
			for deadline := time.Now().Add(time.Second); harness.restarted.Load() == 0 && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			}
			if harness.restarted.Load() != 1 {
				t.Fatal("restart was not requested")
			}
			harness.dependencies.Current, harness.dependencies.PID = scenario.installedVersion, 202
			harness.updater = selfupdate.New(harness.dependencies)
			harness.engine = echo.New()
			manager, bootstrap, _ := session.NewManager(rand.Reader)
			harness.credentials, _, _ = manager.BootstrapForSession(bootstrap, "")
			harness.engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: func() bool { return false }}).Middleware)
			registerUpdateRoutes(harness.engine, &UpdateHandlers{Current: scenario.installedVersion, Service: harness.updater, Actions: ActionHandlers{Sessions: manager}})
			status := awaitWebUpdate(t, harness, "succeeded")
			if status.Current != scenario.installedVersion {
				t.Fatalf("current = %q", status.Current)
			}
			if status.Job.Target != "v1.1.0" || status.Job.InstalledVersion == nil || *status.Job.InstalledVersion != scenario.installedVersion {
				t.Fatalf("installed version = %+v", status.Job)
			}
			denied := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update/preview", []byte(`{"target":"v1.2.0"}`), "")
			if denied.Code != http.StatusConflict || problemCode(t, denied.Body.Bytes()) != "vault_locked" {
				t.Fatal("locked vault permitted another update")
			}
		})
	}
}

func TestWebUpdateRequiresBrowserSessionCSRFAndExactConfirmedInstallation(t *testing.T) {
	harness := newWebUpdateHarness(t)
	preview := harness.preview(t)
	for _, boundary := range []string{"session", "csrf", "origin", "action"} {
		request := httptest.NewRequest(http.MethodPost, "http://"+keyTestHost+"/api/v1/update", bytes.NewBufferString(`{"target":"v1.1.0"}`))
		request.Host = keyTestHost
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("Origin", "http://"+keyTestHost)
		request.Header.Set("Content-Type", "application/json")
		if boundary != "session" {
			request.AddCookie(&http.Cookie{Name: SessionCookie, Value: harness.credentials.SessionID})
		}
		if boundary != "csrf" {
			request.Header.Set(CSRFHeader, harness.credentials.CSRFToken)
		}
		if boundary == "origin" {
			request.Header.Set("Origin", "http://attacker.invalid")
		}
		if boundary != "action" {
			request.Header.Set(ActionHeader, preview.ActionToken)
		}
		response := httptest.NewRecorder()
		harness.engine.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden && response.Code != http.StatusUnauthorized {
			t.Fatalf("%s = %d", boundary, response.Code)
		}
	}
	harness.installation.Identity = "changed-binary-digest"
	refused := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", []byte(`{"target":"v1.1.0"}`), preview.ActionToken)
	if refused.Code != http.StatusForbidden || !strings.Contains(refused.Body.String(), "action_token_invalid") {
		t.Fatalf("changed plan = %d %s", refused.Code, refused.Body.String())
	}
	if harness.installed.Load() != 0 {
		t.Fatal("refused requests ran installation")
	}
}

func TestWebUpdateRejectsExpiredAndOtherSessionConfirmations(t *testing.T) {
	for _, boundary := range []string{"expired", "other session", "other action"} {
		t.Run(boundary, func(t *testing.T) {
			harness := newWebUpdateHarness(t)
			moment := time.Now()
			harness.sessions.Now = func() time.Time { return moment }
			preview := harness.preview(t)
			credentials := harness.credentials
			token := preview.ActionToken
			if boundary == "expired" {
				moment = moment.Add(session.ActionTokenTTL + time.Second)
			}
			if boundary == "other session" {
				credentials, _, _ = harness.sessions.JoinOrIssue("", "")
			}
			if boundary == "other action" {
				token, _ = harness.sessions.IssueAction(credentials.SessionID, session.ActionRequest{Kind: session.ActionRevealPrivateKey, Target: "v1.1.0", Evidence: "irrelevant"})
			}
			response := sendKeyRequest(t, harness.engine, credentials, http.MethodPost, "/api/v1/update", []byte(`{"target":"v1.1.0"}`), token)
			if response.Code != http.StatusForbidden || harness.installed.Load() != 0 {
				t.Fatalf("%s = %d %s", boundary, response.Code, response.Body.String())
			}
		})
	}
}

func TestWebUpdateProgressDoesNotDependOnTheReleaseServer(t *testing.T) {
	harness := newWebUpdateHarness(t)
	var checked atomic.Int32
	releaseServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		checked.Add(1)
		response.WriteHeader(http.StatusBadGateway)
	}))
	defer releaseServer.Close()
	checker := &releasecheck.Checker{API: releaseServer.URL, HTTP: releaseServer.Client()}
	harness.handlers.Watcher = releasewatch.New(context.Background(), func(ctx context.Context) (releasecheck.Catalog, error) { return checker.Catalog(ctx, "v1.0.0") })
	t.Cleanup(harness.handlers.Watcher.Stop)
	preview := harness.preview(t)
	response := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", []byte(`{"target":"v1.1.0"}`), preview.ActionToken)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	awaitWebUpdate(t, harness, "installing")
	if checked.Load() != 0 {
		t.Fatal("progress asked the release server")
	}
	harness.installResult <- errors.New("fixture failure")
	awaitWebUpdate(t, harness, "failed")
}

func readUpdateLog(t *testing.T, harness *webUpdateHarness, query string) (int, api.UpdateLog, string) {
	t.Helper()
	response := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodGet, "/api/v1/update/log"+query, nil, "")
	var log api.UpdateLog
	if response.Code == http.StatusOK {
		if err := json.Unmarshal(response.Body.Bytes(), &log); err != nil {
			t.Fatal(err)
		}
		return response.Code, log, ""
	}
	return response.Code, log, problemCode(t, response.Body.Bytes())
}

func TestWebUpdateLogShowsInstallerOutputBeforeTheInstallerEnds(t *testing.T) {
	harness := newWebUpdateHarness(t)
	harness.installerOutput = "\x1b[32msshc: installing sshc-linux-amd64 (v1.1.0)\x1b[0m\n"
	if code, _, problem := readUpdateLog(t, harness, ""); code != http.StatusNotFound || problem != "update_log_missing" {
		t.Fatalf("log before any job = %d %s", code, problem)
	}
	preview := harness.preview(t)
	accepted := sendKeyRequest(t, harness.engine, harness.credentials, http.MethodPost, "/api/v1/update", []byte(`{"target":"v1.1.0"}`), preview.ActionToken)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("acceptance = %d: %s", accepted.Code, accepted.Body.String())
	}
	var job api.UpdateJob
	if err := json.Unmarshal(accepted.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	awaitWebUpdate(t, harness, "installing")
	var log api.UpdateLog
	for deadline := time.Now().Add(3 * time.Second); log.Output == "" && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		var code int
		code, log, _ = readUpdateLog(t, harness, "?cursor=0")
		if code != http.StatusOK {
			t.Fatalf("log while installing = %d", code)
		}
	}
	// The installer has not returned yet: it still waits for installResult.
	if log.JobId != job.Id || log.Output != "sshc: installing sshc-linux-amd64 (v1.1.0)\n" || log.Cursor.Next != log.Cursor.End || log.ReachedCapacity {
		t.Fatalf("log while installing = %+v", log)
	}
	// Once the update restarts the engine, the vault is locked; the output stays readable.
	harness.locked = true
	if code, rest, _ := readUpdateLog(t, harness, "?cursor="+strconv.FormatUint(log.Cursor.Next, 10)); code != http.StatusOK || rest.Output != "" {
		t.Fatalf("log while the vault is locked = %d %+v", code, rest)
	}
	if code, _, problem := readUpdateLog(t, harness, "?cursor="+strconv.FormatUint(log.Cursor.End+1, 10)); code != http.StatusConflict || problem != "terminal_cursor_ahead" {
		t.Fatalf("cursor past the end = %d %s", code, problem)
	}
	if code, _, problem := readUpdateLog(t, harness, "?limit=65537"); code != http.StatusBadRequest || problem != "invalid_terminal_cursor" {
		t.Fatalf("limit over the maximum = %d %s", code, problem)
	}
	harness.installResult <- nil
	awaitWebUpdate(t, harness, "restarting")
}
