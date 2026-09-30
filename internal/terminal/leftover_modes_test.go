package terminal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/terminal"
)

// マウスの報告を有効にしたまま動いていたプログラム（tmux、htopなど）の出力である。
const mouseReportingProgram = "\x1b[?1003h\x1b[?1006h"

// 次のシェルが自分で有効にするモードとプロンプトである。
const nextShellPrompt = "\x1b[?2004h$ "

// leftoverModeResetFixture は、testdata/leftover-mode-reset.jsonの列を読む。
// 画面の側のテストも、同じ列をxterm.jsへ流して効き方を確かめる。
func leftoverModeResetFixture(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "leftover-mode-reset.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Sequence string `json:"sequence"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Sequence == "" {
		t.Fatal("leftover-mode-reset.json has no sequence")
	}
	return fixture.Sequence
}

// requireInOrder は、parts が output にこの順で現れることを確かめる。
func requireInOrder(t *testing.T, output []byte, parts ...string) {
	t.Helper()
	rest := output
	for _, part := range parts {
		index := bytes.Index(rest, []byte(part))
		if index < 0 {
			t.Fatalf("%q is missing or out of order in %q", part, output)
		}
		rest = rest[index+len(part):]
	}
}

func TestMouseReportingLeftByADisconnectedProgramIsOffBeforeTheReconnectedShellStarts(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).feed(mouseReportingProgram)
	spy.at(0).exit(terminal.ExitInfo{Code: terminal.TransportLost})
	waitFor(t, func() bool { return spy.count() >= 2 })
	spy.at(1).feed(nextShellPrompt)
	waitFor(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

	// 切断の知らせより前に戻すので、知らせも新しいシェルも通常の画面へ出る。新しいシェルが
	// 有効にしたモードは、戻したあとに届くので効いたままになる。
	requireInOrder(t, snapshotOf(session), mouseReportingProgram, reset, "SSH接続が切れました", nextShellPrompt)
}

func TestModesLeftByAnExitedShellAreOffBeforeAManualReconnectStartsANewShell(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).feed(mouseReportingProgram)
	spy.at(0).exit(terminal.ExitInfo{Code: 0})
	waitFor(t, func() bool { return !session.Live() })
	if _, err := registry.Reconnect(context.Background(), session.ID()); err != nil {
		t.Fatal(err)
	}
	spy.at(1).feed(nextShellPrompt)
	waitFor(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

	requireInOrder(t, snapshotOf(session), mouseReportingProgram, reset, nextShellPrompt)
}
