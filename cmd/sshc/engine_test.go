package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		switch {
		case r.URL.Path == "/api/v1/health":
			if !fixture.healthy.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == httpserver.StopPath:
			if r.Header.Get(handoff.HeaderName) != fixture.secret {
				w.WriteHeader(http.StatusForbidden)
				return
			}
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
		context.Background(), []string{"start"}, fixture.stateDir,
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
		context.Background(), []string{"start"}, fixture.stateDir,
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
		context.Background(), []string{"start"}, fixture.stateDir,
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
		context.Background(), []string{"stop"}, fixture.stateDir,
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
		context.Background(), []string{"stop"}, t.TempDir(),
		&http.Client{Timeout: time.Second}, nil, &strings.Builder{}, &errOut)
	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
}

func TestEngineRefusesWordsItDoesNotKnow(t *testing.T) {
	for _, arguments := range [][]string{{}, {"restart"}, {"start", "extra"}} {
		var errOut strings.Builder
		status := runEngineCommand(
			context.Background(), arguments, t.TempDir(),
			&http.Client{Timeout: time.Second}, nil, &strings.Builder{}, &errOut)
		if status != 2 {
			t.Errorf("engine %v = %d, want 2", arguments, status)
		}
	}
}

// **既定では URL を決して表示しない。** 有効な bootstrap トークンを運ぶ。
func TestOpenPrintsTheURLOnlyWhenAsked(t *testing.T) {
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

	var opened []string
	var out, errOut strings.Builder
	status := runOpen(
		context.Background(), stateDir, &http.Client{Timeout: 5 * time.Second},
		func(target string) error { opened = append(opened, target); return nil },
		false, &out, &errOut)
	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if len(opened) != 1 || opened[0] != entry {
		t.Fatalf("opened = %#v", opened)
	}
	if strings.Contains(out.String(), "bootstrap") {
		t.Fatalf("the default printed a live token: %q", out.String())
	}

	opened = nil
	out.Reset()
	status = runOpen(
		context.Background(), stateDir, &http.Client{Timeout: 5 * time.Second},
		func(target string) error { opened = append(opened, target); return nil },
		true, &out, &errOut)
	if status != 0 {
		t.Fatalf("status = %d: %s", status, errOut.String())
	}
	if len(opened) != 0 {
		t.Fatalf("--print-url also opened something: %#v", opened)
	}
	if strings.TrimSpace(out.String()) != entry {
		t.Fatalf("stdout = %q, want exactly the entry", out.String())
	}
}
