package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/sshclient"
)

// fakeProbe は、設定された engine 状態を順番に返す。
type fakeProbe struct {
	mutex       sync.Mutex
	answers     []statusAnswer
	statusCalls int
	statusErr   error
	connections []string
	connection  connectAnswer
}

func TestCLIRechecksPasswordBindingAgainstItsResolvedTarget(t *testing.T) {
	target := sshclient.Target{
		Alias: "edge", HostName: "original.example", Port: "22", User: "deploy",
		Methods: sshclient.DefaultMethods(),
	}
	answer := connectAnswer{
		Passwords:        map[string]string{"edge": "must-not-travel"},
		PasswordBindings: map[string]string{"edge": target.AuthenticationBinding()},
	}
	lookup := savedPasswordFor(answer)
	if password, ok := lookup(target); !ok || password != "must-not-travel" {
		t.Fatalf("matching target = %q, %v", password, ok)
	}
	target.HostName = "retargeted.example"
	if password, ok := lookup(target); ok || password != "" {
		t.Fatalf("retargeted destination received password = %q, %v", password, ok)
	}
}

func (probe *fakeProbe) Status(context.Context) (statusAnswer, error) {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	probe.statusCalls++
	if probe.statusErr != nil {
		return statusAnswer{}, probe.statusErr
	}
	if len(probe.answers) == 0 {
		return statusAnswer{}, errors.New("no scripted answer")
	}
	answer := probe.answers[0]
	if len(probe.answers) > 1 {
		probe.answers = probe.answers[1:]
	}
	return answer, nil
}

func (probe *fakeProbe) Connection(_ context.Context, alias string) (connectAnswer, error) {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	probe.connections = append(probe.connections, alias)
	return probe.connection, nil
}

func (probe *fakeProbe) requestedAliases() []string {
	probe.mutex.Lock()
	defer probe.mutex.Unlock()
	return append([]string(nil), probe.connections...)
}

func unlockedEngine() statusAnswer {
	return statusAnswer{Owner: handoff.OwnerEngine, ProtocolVersion: handoff.ProtocolVersion, Vault: true, Unlocked: true}
}

func lockedEngine() statusAnswer {
	return statusAnswer{Owner: handoff.OwnerEngine, ProtocolVersion: handoff.ProtocolVersion, Vault: true}
}

// stateWithEngine は、handoff だけを置く。応答するのは fakeProbe である。
func stateWithEngine(t *testing.T, owner handoff.Owner) string {
	t.Helper()
	stateDir := t.TempDir()
	document := testHandoff("http://127.0.0.1:1")
	document.Owner = owner
	if err := handoff.Write(stateDir, document); err != nil {
		t.Fatal(err)
	}
	return stateDir
}

func reach(t *testing.T, stateDir string, probe engineProbe, stderr *bytes.Buffer) (engineProbe, error) {
	t.Helper()
	return reachUnlockedEngine(context.Background(), stateDir, &http.Client{},
		func(handoff.Handoff) engineProbe { return probe }, stderr)
}

// reachUnlockedEngine は engine を起動せず、各状態に対応する復旧手順を返す。
func TestReachUnlockedEngineExplainsInsteadOfStartingOne(t *testing.T) {
	for _, test := range []struct {
		name    string
		running bool
		answers []statusAnswer
		wantErr string
	}{
		{
			name:    "an unlocked engine is handed straight over",
			running: true, answers: []statusAnswer{unlockedEngine()},
		},
		{
			name:    "a locked vault says how to unlock it",
			running: true, answers: []statusAnswer{lockedEngine()},
			wantErr: "sshc vault unlock",
		},
		{
			// Vault 未作成と解錠済みを区別する。
			name:    "no vault says how to create one",
			running: true,
			answers: []statusAnswer{{Owner: handoff.OwnerEngine, ProtocolVersion: handoff.ProtocolVersion}},
			wantErr: "sshc vault create",
		},
		{
			name:    "no engine says how to start one, and names ssh as the way through without it",
			running: false,
			wantErr: "sshc engine",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			if test.running {
				stateDir = stateWithEngine(t, handoff.OwnerEngine)
			}
			probe := &fakeProbe{answers: test.answers}
			var stderr bytes.Buffer

			session, err := reach(t, stateDir, probe, &stderr)

			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v, want an unlocked engine", err)
				}
				if session == nil {
					t.Error("session carries no probe")
				}
				return
			}
			if err == nil {
				t.Fatalf("err = nil, want %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("err = %q, want it to name %q", err, test.wantErr)
			}
		})
	}
}

// engine が停止中の場合は、通常の ssh を代替手段として明示する。
func TestTheMissingEngineMessageNamesTheWayThroughWithoutIt(t *testing.T) {
	_, err := reach(t, t.TempDir(), &fakeProbe{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("err = nil, want it to explain")
	}
	if !strings.Contains(err.Error(), "ssh") {
		t.Errorf("err = %q, want it to name ssh as the way through without the engine", err)
	}
}

// 指定された alias の接続情報を一度だけ要求する。
func TestConnectionIsRequestedOnceForTheOriginalAlias(t *testing.T) {
	stateDir := stateWithEngine(t, handoff.OwnerEngine)
	probe := &fakeProbe{answers: []statusAnswer{unlockedEngine()}}

	session, err := reach(t, stateDir, probe, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("reach: %v", err)
	}
	if _, err := session.Connection(context.Background(), "the-original-alias"); err != nil {
		t.Fatalf("connection: %v", err)
	}

	requested := probe.requestedAliases()
	if len(requested) != 1 || requested[0] != "the-original-alias" {
		t.Errorf("requested aliases = %v, want the original alias exactly once", requested)
	}
}
