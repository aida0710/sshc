package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/platform/windowsacl/acltest"
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

// 版の異なる CLI が古い規約で API を叩くと、拒否の原因が見えず利用者だけが
// 取り残される。読み口を一つにすることで、すべての CLI command が同じ復旧策を出す。
//
// **「アプリを再起動してください」とは言わない。** 食い違っているのがどちら側かを
// このプロセスは知らず、**古いのがこちらである状況は設計が自分で作っている**——
// 外殻は `~/.local/bin/sshc` に自分が張ったリンク以外のものを触らないので、
// `make install` で入れた実体はアプリを入れ替えても古いまま残る。だから、いま
// 話しているのがどの実体かを名指しする。
func TestReadHandoffExplainsHowToRecoverFromAProtocolMismatch(t *testing.T) {
	document := testHandoff("http://127.0.0.1:52865")
	document.ProtocolVersion++
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	acltest.WritePrivateFile(t, filepath.Join(stateDir, handoff.FileName), body)

	_, err = readHandoff(stateDir)
	if !errors.Is(err, handoff.ErrProtocolVersion) {
		t.Fatalf("readHandoff = %v, want protocol-version error", err)
	}
	executable, executableErr := os.Executable()
	if executableErr != nil {
		t.Fatal(executableErr)
	}
	if !strings.Contains(err.Error(), "not the same version") ||
		!strings.Contains(err.Error(), "update whichever is older") ||
		!strings.Contains(err.Error(), executable) {
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

// **入れた直後の機械では、handoff がまだ無い。**
//
// そのまま返すと利用者が読むのは `open /home/a/.ssh/sshc/cli: no such file or
// directory` である。入れて最初に打つのが `sshc status` なので、そこが道の綴りを
// 返すのは、案内として一番効く場所を捨てている。
func TestAskingAMachineThatHasNeverRunAnEngineGetsAnAnswerItCanAct(t *testing.T) {
	_, err := readHandoff(t.TempDir())
	if err == nil {
		t.Fatal("a machine with no handoff answered as if an engine were running")
	}
	message := err.Error()
	for _, want := range []string{"not running", "headless"} {
		if !strings.Contains(message, want) {
			t.Errorf("%q does not tell the reader %q", message, want)
		}
	}
	if strings.Contains(message, "no such file") {
		t.Errorf("%q spells a path instead of saying what happened", message)
	}
	// **判定は壊さない。** 文言のために sentinel を失うと、これを
	// errors.Is で見ている呼び出し側が黙って別の枝へ行く。
	if !errors.Is(err, fs.ErrNotExist) {
		t.Error("the friendly message dropped fs.ErrNotExist")
	}
}
