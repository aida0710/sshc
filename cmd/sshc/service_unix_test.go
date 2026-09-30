//go:build !windows

package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/storage"
)

// fakeRestartableService は、restartServiceIfActive が OS ごとの manager に求める操作を
// 記録する。afterStatus は Status のたびに呼ばれ、確認の間に定義が変わる場面を作る。
type fakeRestartableService struct {
	definition     serviceDefinitionFile
	states         []serviceState
	afterStatus    func()
	restarts       int
	readinessWaits int
}

func (service *fakeRestartableService) definitionFile() serviceDefinitionFile {
	return service.definition
}

func (service *fakeRestartableService) acquireOperationLock() (func() error, error) {
	return func() error { return nil }, nil
}

func (service *fakeRestartableService) Status(context.Context) (serviceState, error) {
	state := service.states[0]
	service.states = service.states[1:]
	if service.afterStatus != nil {
		service.afterStatus()
	}
	return state, nil
}

func (service *fakeRestartableService) restartRunning(context.Context) error {
	service.restarts++
	return nil
}

func (service *fakeRestartableService) waitUntilReady(context.Context) error {
	service.readinessWaits++
	return nil
}

func testServiceDefinition(t *testing.T) serviceDefinitionFile {
	t.Helper()
	definition := serviceDefinitionFile{
		files:  storage.OSFileSystem{},
		path:   filepath.Join(t.TempDir(), "sshc.definition"),
		marker: "# managed\n",
		name:   "test service definition",
		render: func(executable string) (string, error) { return "# managed\nrun " + executable + "\n", nil },
	}
	contents, err := definition.render("/opt/sshc/bin/sshc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(definition.path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return definition
}

func TestServiceRestartSkipsADefinitionReplacedWhileTheStateWasChecked(t *testing.T) {
	definition := testServiceDefinition(t)
	service := &fakeRestartableService{definition: definition, states: []serviceState{serviceActive}}
	service.afterStatus = func() {
		if err := os.WriteFile(definition.path, []byte("# managed\nrun /other/sshc\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restarted, err := restartServiceIfActive(context.Background(), service, "/opt/sshc/bin/sshc")
	if err != nil || restarted || service.restarts != 0 {
		t.Fatalf("restart = %v, %v, restarts=%d", restarted, err, service.restarts)
	}
}

func TestServiceRestartDoesNotWaitForAServiceThatStoppedDuringTheRestart(t *testing.T) {
	service := &fakeRestartableService{definition: testServiceDefinition(t), states: []serviceState{serviceActive, serviceInactive}}
	restarted, err := restartServiceIfActive(context.Background(), service, "/opt/sshc/bin/sshc")
	if err != nil || restarted || service.restarts != 1 || service.readinessWaits != 0 {
		t.Fatalf("restart = %v, %v, restarts=%d, readiness waits=%d", restarted, err, service.restarts, service.readinessWaits)
	}
}

func TestServiceRestartFailsWhenTheDefinitionChangesWhileTheEngineStarts(t *testing.T) {
	definition := testServiceDefinition(t)
	service := &fakeRestartableService{definition: definition, states: []serviceState{serviceActive, serviceActive}}
	statusChecks := 0
	service.afterStatus = func() {
		statusChecks++
		if statusChecks == 2 {
			if err := os.WriteFile(definition.path, []byte("# managed\nrun /other/sshc\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	restarted, err := restartServiceIfActive(context.Background(), service, "/opt/sshc/bin/sshc")
	if restarted || err == nil || !strings.Contains(err.Error(), "test service definition changed while the service was restarting") {
		t.Fatalf("restart = %v, %v", restarted, err)
	}
}

// service の準備待ちも、challenge に答えられない相手には handoff の秘密を送らない。
func TestServiceReadinessDoesNotSendTheSecretToAnUnprovenEngine(t *testing.T) {
	var secretSent atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get(handoff.HeaderName) != "" {
			secretSent.Store(true)
		}
		if request.URL.Path == httpserver.ChallengePath {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(response, `{"owner":"engine","version":"test","protocolVersion":1,"vault":false,"unlocked":false,"sessions":0}`)
	}))
	defer server.Close()
	home := t.TempDir()
	writeTestHandoff(t, app.HandoffDir(home), server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*serviceReadyPollInterval)
	defer cancel()
	err := waitForEngineReady(ctx, home, engineReadiness{
		mainPID:       func(context.Context) int { return testHandoff("").PID },
		failureDetail: func(context.Context) string { return "" },
	})
	if err == nil {
		t.Fatal("an engine that could not prove the handoff secret was reported ready")
	}
	if secretSent.Load() {
		t.Fatal("the handoff secret was sent before the engine proved it")
	}
}
