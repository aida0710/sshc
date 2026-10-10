//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertServiceRestartPlanProtectsDefinitions(t *testing.T, manager engineServiceManager, definition serviceDefinitionFile) {
	t.Helper()
	current, err := definition.render("/opt/sshc/bin/sshc")
	if err != nil {
		t.Fatal(err)
	}
	other, err := definition.render("/other/sshc")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(definition.path), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		contents string
		want     error
	}{
		{name: "current", contents: current},
		{name: "absent", want: errServiceNotInstalled},
		{name: "unmanaged", contents: "hand-written definition\n", want: errUnmanagedServiceUnit},
		{name: "outdated", contents: current + "\n", want: errOutdatedServiceDefinition},
		{name: "another installation", contents: other, want: errServiceExecutableMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.Remove(definition.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if test.contents != "" {
				if err := os.WriteFile(definition.path, []byte(test.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := manager.RestartPlan("/opt/sshc/bin/sshc")
			if !errors.Is(err, test.want) {
				t.Fatalf("plan=%q error=%v, want %v", plan, err, test.want)
			}
			if err == nil && (!strings.Contains(plan, definition.path) || !strings.Contains(plan, "/opt/sshc/bin/sshc") || !strings.Contains(plan, "connections and transfers will be interrupted")) {
				t.Fatalf("plan does not explain the restart: %q", plan)
			}
			if err != nil && plan != "" {
				t.Fatalf("refused service still produced a plan: %q", plan)
			}
			contents, readErr := os.ReadFile(definition.path)
			if test.contents == "" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("missing definition was created: %v", readErr)
				}
			} else if readErr != nil || !bytes.Equal(contents, []byte(test.contents)) {
				t.Fatalf("definition changed: %q, %v", contents, readErr)
			}
		})
	}
}

func TestServiceRestartWaitsForReadinessAndReleasesTheOperationLock(t *testing.T) {
	for _, readinessErr := range []error{nil, errors.New("engine lock is held by another process")} {
		name := "ready"
		if readinessErr != nil {
			name = "readiness failed"
		}
		t.Run(name, func(t *testing.T) {
			lock := serviceOperationLock(t.TempDir())
			service := &fakeRestartableService{
				definition: testServiceDefinition(t), states: []serviceState{serviceActive, serviceActive},
				lock: lock, readinessErr: readinessErr,
			}
			release, err := lock()
			if err != nil {
				t.Fatal(err)
			}
			restarted, restartErr := restartServiceIfActive(context.Background(), service, "/opt/sshc/bin/sshc")
			releaseErr := release()
			if restarted || restartErr == nil || service.restarts != 0 || releaseErr != nil {
				t.Fatalf("concurrent restart=%v error=%v restarts=%d release=%v", restarted, restartErr, service.restarts, releaseErr)
			}
			restarted, err = restartServiceIfActive(context.Background(), service, "/opt/sshc/bin/sshc")
			if restarted != (readinessErr == nil) || !errors.Is(err, readinessErr) || service.restarts != 1 || service.readinessWaits != 1 {
				t.Fatalf("restart=%v error=%v restarts=%d readiness waits=%d", restarted, err, service.restarts, service.readinessWaits)
			}
			release, err = lock()
			if err != nil {
				t.Fatalf("restart kept the operation lock: %v", err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
