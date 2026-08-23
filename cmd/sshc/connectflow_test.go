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
)

// fakeProbe は、生きているエンジンの答えを台本どおりに返す。
type fakeProbe struct {
	mutex       sync.Mutex
	answers     []statusAnswer
	statusCalls int
	statusErr   error
	connections []string
	connection  connectAnswer
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

// **このコマンドは engine を起こさない。** 生かしておくのは人であり、ここで
// できるのは、居ないことと起こし方を言うことだけである。だから四つの出口は
// すべて「繋ぐ」か「次に打つものを言う」のどちらかで終わる——待ちは無い。
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
			// **保管庫の不在は解錠ではない。** 無ければ保存された答えは一つも無い。
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

// **黙って退かない。** このアプリケーションが ~/.ssh/config に触れないので ssh は
// 常に動くが、黙って退けば、鍵のパスフレーズを毎回訊かれるのが engine の不在の
// せいだと分からないままになる。
func TestTheMissingEngineMessageNamesTheWayThroughWithoutIt(t *testing.T) {
	_, err := reach(t, t.TempDir(), &fakeProbe{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("err = nil, want it to explain")
	}
	if !strings.Contains(err.Error(), "ssh") {
		t.Errorf("err = %q, want it to name ssh as the way through without the engine", err)
	}
}

// 繋ぐときは、利用者に打ち直させない。元の接続先をそのまま一回要求する。
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
