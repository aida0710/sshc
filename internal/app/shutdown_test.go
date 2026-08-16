package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/terminal"
)

// stuckProcess は、Hangup の中で返らない相手である。応答しないリモートに向いた
// ssh が、まさにこの形になる。
type stuckProcess struct {
	release chan struct{}
	forced  atomic.Bool
	once    sync.Once
}

func newStuckProcess() *stuckProcess { return &stuckProcess{release: make(chan struct{})} }

func (p *stuckProcess) Read([]byte) (int, error) {
	<-p.release
	return 0, errors.New("closed")
}
func (p *stuckProcess) Write(b []byte) (int, error) { return len(b), nil }
func (p *stuckProcess) Resize(terminal.Size) error  { return nil }

// Hangup は返らない。**これが停止処理の本当の相手である。**
func (p *stuckProcess) Hangup() error { <-p.release; return nil }

func (p *stuckProcess) Wait() terminal.ExitInfo { <-p.release; return terminal.ExitInfo{Code: -1} }
func (p *stuckProcess) Close() error            { return nil }

func (p *stuckProcess) ForceClose() error {
	p.forced.Store(true)
	p.once.Do(func() { close(p.release) })
	return nil
}

func testDependencies(t *testing.T) Dependencies {
	t.Helper()
	return Dependencies{
		Home:   t.TempDir(),
		Random: bytes.NewReader(bytes.Repeat([]byte{0x31}, 512)),
		Listen: net.Listen,
		UI:     fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:  handoff.OwnerHeadless,
		PID:    4242,
		// 秒を待たない。締切そのものが検査対象であって、その長さではない。
		ShutdownTimeout: 40 * time.Millisecond,
	}
}

// **締切は、後始末がどこまで進んだかと無関係に張られる。**
//
// 返らない Hangup を抱えたセッションが一本あっても、その締切に達したら強制停止は
// 始まる。そして合流は、それが本当に終わるまで返らない——engine lock を手放すのは
// この後だからである。
func TestUnwindForcesABlockedTerminalAtTheDeadline(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()

	process := newStuckProcess()
	if _, err := built.terminals.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "unreachable",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	}); err != nil {
		t.Fatal(err)
	}

	unwound := make(chan error, 1)
	started := time.Now()
	go func() { unwound <- built.unwind(dependencies) }()

	select {
	case err := <-unwound:
		if err != nil {
			t.Fatalf("unwind = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("unwind never returned; the deadline did not force the blocked terminal")
	}
	if !process.forced.Load() {
		t.Fatal("the blocked terminal was never forced")
	}
	if elapsed := time.Since(started); elapsed < dependencies.ShutdownTimeout {
		t.Fatalf("unwind returned in %v, before its own %v deadline", elapsed, dependencies.ShutdownTimeout)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

// 後始末は、どの段が失敗しても後ろの段を走らせる。ここでは名簿がもう無い状態から
// 始めて、それでも合流と施錠と listener の停止が起きることを見る。
func TestUnwindContinuesAfterAFailedHandoffRemoval(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()

	// 誰かが先に消した名簿。削除は失敗するが、止まってはならない。
	if err := handoff.Remove(HandoffDir(dependencies.Home), built.document.Secret); err != nil {
		t.Fatal(err)
	}

	if err := built.unwind(dependencies); err != nil {
		t.Fatalf("unwind = %v", err)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

// 停止を始めた server は、状態を変える要求を断る。
//
// **ただし門は Security の後ろにある。** 無効な Host や origin の要求は、停止中
// かどうかに関わらず今までどおり弾かれ、入場の数にも入らない。この順序は意図で
// あり、ミドルウェアの並べ替えで静かに変わってはならない。
func TestStoppingRefusesValidMutationsAfterSecurityHasAcceptedThem(t *testing.T) {
	dependencies := testDependencies(t)
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()
	t.Cleanup(func() {
		built.server.BeginShutdown()
		_ = built.server.Wait()
		<-served
	})

	base := built.server.URL()
	client := &http.Client{Timeout: 5 * time.Second}
	post := func(t *testing.T, valid bool) int {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, base+"/api/v1/session/bootstrap", nil)
		if err != nil {
			t.Fatal(err)
		}
		if valid {
			request.Header.Set("Origin", base)
			request.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}

	// 停止前は、門を通って本来のハンドラへ届く。ブートストラップの資格情報が
	// 無いので受理はされないが、503 ではない。
	if status := post(t, true); status == http.StatusServiceUnavailable {
		t.Fatal("a mutation was refused as stopping before stopping began")
	}

	built.server.BeginStopping()

	if status := post(t, true); status != http.StatusServiceUnavailable {
		t.Fatalf("valid mutation during stopping = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if status := post(t, false); status != http.StatusForbidden {
		t.Fatalf("invalid-origin mutation during stopping = %d, want %d", status, http.StatusForbidden)
	}

	// 読み取りは、停止中でも門で断られない。
	health, err := http.NewRequest(http.MethodGet, base+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	health.Header.Set("Origin", base)
	health.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := client.Do(health)
	if err != nil {
		t.Fatal(err)
	}
	status := response.StatusCode
	response.Body.Close()
	if status == http.StatusServiceUnavailable {
		t.Fatal("a read-only request was refused by the stopping gate")
	}
}
