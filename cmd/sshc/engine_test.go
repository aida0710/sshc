package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// engineFixture は、handoff を書いた state ディレクトリと、そこが指すサーバー。
type engineFixture struct {
	stateDir string
	secret   string
	url      string
	stops    atomic.Int64
	healthy  atomic.Bool
}

// engineStarts は、起こされたエンジンがすることを真似る。
//
// **本物は handoff を書く。** runEngineStart は死んだ handoff を消してから
// 起こすので、書き直さない偽物では「起きたのに見つからない」になる。
func (f *engineFixture) engineStarts(t *testing.T) {
	t.Helper()
	if _, err := handoff.Write(f.stateDir, f.url, f.secret); err != nil {
		t.Error(err)
		return
	}
	f.healthy.Store(true)
}

func newEngineFixture(t *testing.T, healthy bool) *engineFixture {
	t.Helper()
	fixture := &engineFixture{stateDir: t.TempDir(), secret: "the secret for this run"}
	fixture.healthy.Store(healthy)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// **秘密を持たない者には答えない。** 本物がそうなので、偽物もそうする
		// ——偽物が本物より寛容だと、この検査は製品が壊れていても緑のままになる。
		// 一度そうなった。
		if r.Header.Get(handoff.HeaderName) != fixture.secret {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == httpserver.HealthPath:
			if !fixture.healthy.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == httpserver.StopPath:
			fixture.stops.Add(1)
			w.WriteHeader(http.StatusAccepted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	fixture.url = server.URL
	if _, err := handoff.Write(fixture.stateDir, server.URL, fixture.secret); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// **既に居るなら起こさない。** 二つ動くと、どちらが handoff を書いたかで
// 接続先が変わる。
func TestEngineStartDoesNotSpawnWhenOneIsAlreadyAnswering(t *testing.T) {
	fixture := newEngineFixture(t, true)
	spawned := 0
	var out, errOut strings.Builder

	status := runEngineCommand(
		context.Background(), []string{"start"}, t.TempDir(), fixture.stateDir,
		&http.Client{Timeout: 5 * time.Second},
		func() error { spawned++; return nil }, &out, &errOut)

	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if spawned != 0 {
		t.Fatalf("a live engine was started %d more time(s)", spawned)
	}
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "http://") {
		t.Errorf("stdout = %q, want the engine's address", out.String())
	}
}

// 死んだ handoff は消してから起こす。**残しておくと、次に読む者が誰も
// 居ないアドレスへ向かう。**
func TestEngineStartReplacesAStaleHandoff(t *testing.T) {
	fixture := newEngineFixture(t, false)
	var out, errOut strings.Builder

	spawned := 0
	status := runEngineCommand(
		context.Background(), []string{"start"}, t.TempDir(), fixture.stateDir,
		&http.Client{Timeout: time.Second},
		func() error {
			spawned++
			fixture.engineStarts(t)
			return nil
		}, &out, &errOut)

	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if spawned != 1 {
		t.Fatalf("the engine was started %d time(s), want once", spawned)
	}
}

// **待つのは handoff が書かれることであって、起こしたプロセスの終了ではない。**
func TestEngineStartWaitsForTheEngineToAnswer(t *testing.T) {
	fixture := newEngineFixture(t, false)
	var out, errOut strings.Builder

	status := runEngineCommand(
		context.Background(), []string{"start"}, t.TempDir(), fixture.stateDir,
		&http.Client{Timeout: time.Second},
		func() error {
			go func() {
				time.Sleep(300 * time.Millisecond)
				fixture.engineStarts(t)
			}()
			return nil
		}, &out, &errOut)

	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Error("nothing was printed after the engine answered")
	}
}

// 止めるのは handoff の秘密を持つ者だけである。
func TestEngineStopPresentsTheHandoffSecret(t *testing.T) {
	fixture := newEngineFixture(t, true)
	var errOut strings.Builder

	status := runEngineCommand(
		context.Background(), []string{"stop"}, t.TempDir(), fixture.stateDir,
		&http.Client{Timeout: 5 * time.Second}, nil, &strings.Builder{}, &errOut)

	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if fixture.stops.Load() != 1 {
		t.Fatalf("the engine was asked to stop %d time(s)", fixture.stops.Load())
	}
}

// 居ないものを止めるのは、要求としては成功である。
func TestEngineStopSucceedsWhenNothingIsRunning(t *testing.T) {
	var errOut strings.Builder
	status := runEngineCommand(
		context.Background(), []string{"stop"}, t.TempDir(), t.TempDir(),
		&http.Client{Timeout: time.Second}, nil, &strings.Builder{}, &errOut)
	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
}

func TestEngineRefusesWordsItDoesNotKnow(t *testing.T) {
	for _, arguments := range [][]string{{}, {"restart"}, {"start", "extra"}} {
		var errOut strings.Builder
		status := runEngineCommand(
			context.Background(), arguments, t.TempDir(), t.TempDir(),
			&http.Client{Timeout: time.Second}, nil, &strings.Builder{}, &errOut)
		if status != 2 {
			t.Errorf("engine %v = %d, want 2", arguments, status)
		}
	}
}

// **開く相手はもう居ない。** このコマンドの仕事は「入口をひとつ発行して
// 渡す」ことだけである。渡す先は、自分でそれを開く親プロセスか、それを読む人で
// ある。
func TestOpenPrintsTheEntranceAndOpensNothing(t *testing.T) {
	entry := "http://127.0.0.1:1/#bootstrap=a-live-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != httpserver.OpenPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": entry})
	}))
	defer server.Close()

	stateDir := t.TempDir()
	if _, err := handoff.Write(stateDir, server.URL, "secret"); err != nil {
		t.Fatal(err)
	}

	var out, errOut strings.Builder
	status := runOpen(context.Background(), stateDir,
		&http.Client{Timeout: 5 * time.Second}, &out, &errOut)
	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if strings.TrimSpace(out.String()) != entry {
		t.Fatalf("stdout = %q, want exactly the entrance", out.String())
	}
}

// **止めるかどうかを決めるのは設定である。** 外殻がそれを読むと、metadata の
// 形を知る場所が二つになる。
func TestEngineQuitHonoursTheKeepRunningSetting(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata string
		wantStop int64
	}{
		{name: "no metadata at all", wantStop: 1},
		{name: "no desktop section", metadata: `{"schemaVersion":3}`, wantStop: 1},
		{name: "asked to stop", metadata: `{"schemaVersion":3,"desktop":{"keepRunning":false}}`, wantStop: 1},
		{name: "asked to keep", metadata: `{"schemaVersion":3,"desktop":{"keepRunning":true}}`, wantStop: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newEngineFixture(t, true)
			home := t.TempDir()
			if test.metadata != "" {
				stateDir := filepath.Join(home, ".ssh", "sshc")
				if err := os.MkdirAll(stateDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(stateDir, "metadata.json"), []byte(test.metadata), 0o600,
				); err != nil {
					t.Fatal(err)
				}
			}

			var errOut strings.Builder
			status := runEngineCommand(
				context.Background(), []string{"quit"}, home, fixture.stateDir,
				&http.Client{Timeout: 5 * time.Second}, nil, &strings.Builder{}, &errOut)
			if status != 0 {
				t.Fatalf("status = %d: %s", status, errOut.String())
			}
			if fixture.stops.Load() != test.wantStop {
				t.Fatalf("the engine was asked to stop %d time(s), want %d",
					fixture.stops.Load(), test.wantStop)
			}
		})
	}
}

// **二つのアプリが同時に起動しても、エンジンはひとつでなければならない。**
//
// 二つ動くと、あとから handoff を書いた方が勝ち、先に繋いだ画面は誰も見て
// いないエンジンを見続ける。
func TestConcurrentStartsProduceOneEngine(t *testing.T) {
	fixture := newEngineFixture(t, false)
	var spawns atomic.Int64

	var wait sync.WaitGroup
	for attempt := 0; attempt < 6; attempt++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			runEngineCommand(
				context.Background(), []string{"start"}, t.TempDir(), fixture.stateDir,
				&http.Client{Timeout: 5 * time.Second},
				func() error {
					spawns.Add(1)
					fixture.engineStarts(t)
					return nil
				}, io.Discard, io.Discard)
		}()
	}
	wait.Wait()

	if spawns.Load() != 1 {
		t.Fatalf("six simultaneous starts produced %d engines, want one", spawns.Load())
	}
}
