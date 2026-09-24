package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"sshc/internal/connectionlog"
)

// recordedLog は、接続ログに書かれた行を覚える。
type recordedLog struct {
	level connectionlog.Level
	lines []string
}

func (log *recordedLog) Enabled(level connectionlog.Level) bool { return level <= log.level }

func (log *recordedLog) Write(_ connectionlog.Level, message string) {
	log.lines = append(log.lines, message)
}

// sshc vpn logs の出力から、sshcエンジンの記録の行だけを取り出す。
func TestTheEngineRecordIsTakenFromTheLogs(t *testing.T) {
	lines := engineRecordLines("== sshcエンジンの記録 ==\n12:00:00 [debug1] 起動します\n\n12:00:01 [debug2] 失敗\n\n== コンテナのログ ==\n別の行\n")

	if strings.Join(lines, "|") != "12:00:00 [debug1] 起動します|12:00:01 [debug2] 失敗" {
		t.Fatalf("lines = %q", lines)
	}
}

// CLI の接続が VPN 経路で失敗したら、sshcエンジンの記録を debug2 に写す。
func TestAFailedRouteCopiesTheEngineRecordIntoTheConnectionLog(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"lines":"== sshcエンジンの記録 ==\n12:00:00 [debug2] docker build の出力：\n12:00:00 [debug2]   ERROR: failed to solve\n\n== コンテナのログ ==\n"}`))
	})
	defer server.Close()
	log := &recordedLog{level: connectionlog.Detailed}
	ctx := connectionlog.With(context.Background(), log)

	copyEngineRecord(ctx, engineRecordRequest{stateDir: stateDir, client: server.Client(), profile: "lab"},
		connectionlog.Detailed)

	if strings.Join(harness.paths, ",") != "/api/v1/vpn/profiles/lab/logs" {
		t.Fatalf("paths = %v", harness.paths)
	}
	if text := strings.Join(log.lines, "\n"); !strings.Contains(text, "ERROR: failed to solve") {
		t.Fatalf("lines = %q", text)
	}
}

// 書かない深さでは、記録を取り寄せない。
func TestTheEngineRecordIsNotFetchedWhenItWouldNotBeShown(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"lines":""}`))
	})
	defer server.Close()
	ctx := connectionlog.With(context.Background(), &recordedLog{level: connectionlog.Brief})

	copyEngineRecord(ctx, engineRecordRequest{stateDir: stateDir, client: server.Client(), profile: "lab"},
		connectionlog.Detailed)

	if len(harness.paths) != 0 {
		t.Fatalf("paths = %v", harness.paths)
	}
}
