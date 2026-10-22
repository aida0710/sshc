package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
)

// fakeRemoteEngines は、決まった URL か決まった失敗を返す。
type fakeRemoteEngines struct {
	url string
	err error
}

func (f fakeRemoteEngines) Open(context.Context, string) (string, error) { return f.url, f.err }

func openRemoteEngine(t *testing.T, opener RemoteEngineOpener, body string) (int, map[string]any) {
	t.Helper()
	engine := connectEngine(t, CLIHandlers{Secret: "secret", RemoteEngines: opener})
	recorder := send(t, engine, http.MethodPost, RemoteEnginePath, body,
		map[string]string{handoff.HeaderName: "secret", "Content-Type": "application/json"})
	var decoded map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("body %q: %v", recorder.Body.String(), err)
	}
	return recorder.Code, decoded
}

func TestOpeningARemoteEngineAnswersWithTheRelayURL(t *testing.T) {
	status, body := openRemoteEngine(t, fakeRemoteEngines{url: "http://127.0.0.1:61000/#bootstrap=x"}, `{"alias":"r540"}`)
	if status != http.StatusOK || body["url"] != "http://127.0.0.1:61000/#bootstrap=x" {
		t.Fatalf("= %d %v", status, body)
	}
}

func TestOpeningARemoteEngineRefusesAnAliasItWouldNotConnectTo(t *testing.T) {
	status, body := openRemoteEngine(t, fakeRemoteEngines{}, `{"alias":"-oProxyCommand=x"}`)
	if status != http.StatusBadRequest || body["code"] != "unsafe_alias" {
		t.Fatalf("= %d %v", status, body)
	}
}

func TestEachReasonARemoteEngineCannotBeOpenedHasItsOwnCode(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		reason any
	}{
		{&remoteengine.ConnectError{Err: fmt.Errorf("hop: %w", sshclient.ErrHostKeyUnknown)}, "remote_engine_connect_failed", "host_key_unknown"},
		{&remoteengine.ConnectError{Err: sshclient.ErrPromptUnavailable}, "remote_engine_connect_failed", "prompt_unavailable"},
		{&remoteengine.ConnectError{Err: errors.New("connection refused")}, "remote_engine_connect_failed", "connection_failed"},
		{remoteengine.ErrNotInstalled, "remote_engine_not_installed", nil},
		{remoteengine.ErrNoEntrance, "remote_engine_no_entrance", nil},
		{fmt.Errorf("%w: prohibited", remoteengine.ErrForwardingRefused), "remote_engine_forwarding_refused", nil},
	} {
		status, body := openRemoteEngine(t, fakeRemoteEngines{err: tc.err}, `{"alias":"r540"}`)
		if status != http.StatusBadGateway || body["code"] != tc.code || body["reason"] != tc.reason {
			t.Errorf("%v = %d %v, want 502 %s %v", tc.err, status, body, tc.code, tc.reason)
		}
	}
}
