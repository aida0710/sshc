package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// **秘密を持たない者には答えない。** 本物がそうなので、偽物もそうする
// ——偽物が本物より寛容だと、この検査は製品が壊れていても緑のままになる。
func TestEngineStatusReadsUnlockedAndSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(handoff.HeaderName) != "the secret" || r.URL.Path != httpserver.StatusPath {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"vault": true, "unlocked": true, "sessions": 3})
	}))
	defer server.Close()

	stateDir := t.TempDir()
	writeTestHandoff(t, stateDir, server.URL)

	answer, err := engineStatus(context.Background(), stateDir, &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("engineStatus: %v", err)
	}
	if !answer.Vault || !answer.Unlocked || answer.Sessions != 3 {
		t.Fatalf("answer = %+v, want an unlocked vault with 3 sessions", answer)
	}
}

// **無い錠の鍵は尋ねない。** 保管庫を作っていないエンジンは「解錠されていない」
// と答えるが、それは施錠されているという意味ではない。ここを取り違えると、
// 新規インストール直後の利用者が接続のたびにマスターパスワードを訊かれる。
func TestLockedOnlyWhenThereIsAVaultToOpen(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer map[string]any
		want   bool
	}{
		{name: "no vault at all", answer: map[string]any{"vault": false, "unlocked": false}, want: false},
		{name: "a vault, still locked", answer: map[string]any{"vault": true, "unlocked": false}, want: true},
		{name: "a vault, already open", answer: map[string]any{"vault": true, "unlocked": true}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(test.answer)
			}))
			defer server.Close()

			stateDir := t.TempDir()
			writeTestHandoff(t, stateDir, server.URL)

			got := locked(context.Background(), stateDir, &http.Client{Timeout: 5 * time.Second})
			if got != test.want {
				t.Fatalf("locked = %v, want %v", got, test.want)
			}
		})
	}
}

// unlock は、開いたかどうかしか返さない。204 は成功、403（間違ったマスター
// パスワード）は失敗として読める必要がある。
func TestUnlockReadsSuccessAndFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		want   bool
	}{
		{name: "the engine unlocked", status: http.StatusNoContent, want: true},
		{name: "the wrong master password", status: http.StatusForbidden, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(handoff.HeaderName) != "the secret" || r.URL.Path != httpserver.UnlockPath {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(test.status)
			}))
			defer server.Close()

			stateDir := t.TempDir()
			writeTestHandoff(t, stateDir, server.URL)

			got := unlock(context.Background(), stateDir, &http.Client{Timeout: 5 * time.Second}, "typed")
			if got != test.want {
				t.Fatalf("unlock = %v, want %v", got, test.want)
			}
		})
	}
}

// 版の異なる CLI が古い規約で API を叩くと、拒否の原因が見えず利用者だけが
// 取り残される。読み口を一つにすることで、すべての CLI command が同じ復旧策を出す。
func TestReadHandoffExplainsHowToRecoverFromAProtocolMismatch(t *testing.T) {
	stateDir := t.TempDir()
	document := testHandoff("http://127.0.0.1:52865")
	document.ProtocolVersion++
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, handoff.FileName), body, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = readHandoff(stateDir)
	if !errors.Is(err, handoff.ErrProtocolVersion) {
		t.Fatalf("readHandoff = %v, want protocol-version error", err)
	}
	if !strings.Contains(err.Error(), "same version") || !strings.Contains(err.Error(), "restart the app") {
		t.Errorf("readHandoff advice = %q", err)
	}
}

func writeTestHandoff(t *testing.T, directory, target string) {
	t.Helper()
	if err := handoff.Write(directory, testHandoff(target)); err != nil {
		t.Fatal(err)
	}
}

func testHandoff(target string) handoff.Handoff {
	return handoff.Handoff{
		SchemaVersion:   handoff.SchemaVersion,
		URL:             target,
		Secret:          "the secret",
		Owner:           handoff.OwnerHeadless,
		PID:             4242,
		Version:         "test",
		ProtocolVersion: handoff.ProtocolVersion,
	}
}

// **メニューバーが読む口である。** エンジンに繋がらないとき、人向けの文言では
// なく非 0 の終了コードで応える必要がある——読むのはコードだけだからだ。
func TestRunStatusFailsWhenTheEngineIsNotThere(t *testing.T) {
	var out, errOut strings.Builder
	status := runStatus(context.Background(), t.TempDir(),
		&http.Client{Timeout: 5 * time.Second}, &out, &errOut)
	if status != 1 {
		t.Fatalf("status = %d, want 1", status)
	}
	if out.String() != "" {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
}
