package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/connectionlog"
	"sshc/internal/vpn"
)

// 経路を起こすのを待つあいだ、時間のかかる段階に入ったら、接続ログの設定に
// 関係なく1度だけ知らせる。ほかのプロファイルの段階は知らせない。
func TestWaitingForARouteAnnouncesItsLongPhasesOnce(t *testing.T) {
	phases := []string{"", "image", "image", "container", "approval", "approval"}
	var mutex sync.Mutex
	served := 0
	allServed := make(chan struct{})
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		phase := phases[min(served, len(phases)-1)]
		served++
		if served == len(phases) {
			close(allServed)
		}
		mutex.Unlock()
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(response, `{"available":true,"profiles":[`+
			`{"profile":{"name":"other"},"running":false,"relaySocket":"","connections":[],"phase":"approval"},`+
			`{"profile":{"name":"lab"},"running":false,"relaySocket":"","connections":[],"phase":%q}]}`, phase)
	})
	defer server.Close()
	engine, err := openEngineAPI(context.Background(), stateDir, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = engine.Close() }()
	log := &recordedLog{level: connectionlog.Notice}
	ctx, cancel := context.WithCancel(connectionlog.With(context.Background(), log))
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		announcePhases(ctx, phaseWatch{engine: engine, profile: "lab", interval: time.Millisecond})
	}()
	<-allServed
	cancel()
	<-finished

	want := vpn.PhaseImage.Notice() + "|" + vpn.PhaseApproval.Notice()
	if got := strings.Join(log.lines, "|"); got != want {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}
