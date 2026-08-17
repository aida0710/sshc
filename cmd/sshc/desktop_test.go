package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// countingLauncher は、何回起こしたかだけを覚える。
type countingLauncher struct {
	available    bool
	availableErr error
	launchErr    error
	launches     int
}

func (launcher *countingLauncher) Available() (bool, error) {
	return launcher.available, launcher.availableErr
}

func (launcher *countingLauncher) Launch(context.Context) error {
	launcher.launches++
	return launcher.launchErr
}

// liveEngine は、status を答える engine を一つ立て、その handoff を書く。
func liveEngine(t *testing.T, owner handoff.Owner) string {
	t.Helper()
	secret := "the secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != httpserver.StatusPath || request.Header.Get(handoff.HeaderName) != secret {
			writer.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(writer).Encode(statusAnswer{Owner: owner, Version: version})
	}))
	t.Cleanup(server.Close)

	stateDir := t.TempDir()
	document := testHandoff(server.URL)
	document.Owner = owner
	if err := handoff.Write(stateDir, document); err != nil {
		t.Fatalf("write handoff: %v", err)
	}
	return stateDir
}

func TestRunDesktopRefusesToDisplaceAHeadlessOwner(t *testing.T) {
	stateDir := liveEngine(t, handoff.OwnerHeadless)
	launcher := &countingLauncher{available: true}
	var stderr bytes.Buffer

	code := runDesktop(context.Background(), stateDir, &http.Client{}, launcher, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if launcher.launches != 0 {
		t.Errorf("launches = %d, want 0; a headless owner must not be displaced", launcher.launches)
	}
	if !strings.Contains(stderr.String(), "headless") {
		t.Errorf("stderr = %q, want the headless owner named", stderr.String())
	}
}

func TestRunDesktopFocusesALiveDesktopExactlyOnce(t *testing.T) {
	stateDir := liveEngine(t, handoff.OwnerDesktop)
	launcher := &countingLauncher{available: true}

	code := runDesktop(context.Background(), stateDir, &http.Client{}, launcher, &bytes.Buffer{})

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if launcher.launches != 1 {
		t.Errorf("launches = %d, want exactly 1", launcher.launches)
	}
}

func TestRunDesktopLaunchesThroughAStaleHandoff(t *testing.T) {
	// engine は死に、handoff だけが残っている。生死を判定しに行かないのは、
	// どちらでもすることが同じだからである。
	stateDir := liveEngine(t, handoff.OwnerDesktop)
	launcher := &countingLauncher{available: true}

	code := runDesktop(context.Background(), stateDir, &http.Client{
		Transport: refusingTransport{},
	}, launcher, &bytes.Buffer{})

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if launcher.launches != 1 {
		t.Errorf("launches = %d, want exactly 1", launcher.launches)
	}
}

func TestRunDesktopSendsAHeadlessUserToHeadless(t *testing.T) {
	launcher := &countingLauncher{available: false}
	var stderr bytes.Buffer

	code := runDesktop(context.Background(), t.TempDir(), &http.Client{}, launcher, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if launcher.launches != 0 {
		t.Errorf("launches = %d, want 0", launcher.launches)
	}
	if !strings.Contains(stderr.String(), "sshc headless") {
		t.Errorf("stderr = %q, want the headless command named", stderr.String())
	}
}

func TestRunDesktopReportsOnlyTheRecordedRepair(t *testing.T) {
	launcher := &countingLauncher{
		availableErr: errors.New("the recorded sshc desktop application is no longer at /old/sshc.AppImage; open the AppImage once at its new location"),
	}
	var stderr bytes.Buffer

	code := runDesktop(context.Background(), t.TempDir(), &http.Client{}, launcher, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if launcher.launches != 0 {
		t.Errorf("launches = %d, want 0", launcher.launches)
	}
	message := stderr.String()
	if !strings.Contains(message, "/old/sshc.AppImage") {
		t.Errorf("stderr = %q, want the recorded absolute target named", message)
	}
	// PATH を引き直せば、利用者が置いていない実体を起こしうる。案内でも勧めない。
	if strings.Contains(message, "PATH") {
		t.Errorf("stderr = %q, must not send the user to a PATH search", message)
	}
	// **直し方を出したら終わり、ではない。** そのアプリケーションを入れる気の
	// 無い人に「入れろ」としか言わないことになる。窓を開けないと分かった時点で、
	// 窓なしで engine を持つ方法も渡す。
	if !strings.Contains(message, "sshc headless") {
		t.Errorf("stderr = %q, want the headless path offered alongside the repair", message)
	}
}

type refusingTransport struct{}

func (refusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}
