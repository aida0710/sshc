package terminal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
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
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() >= 2 })
	spy.at(1).feed(nextShellPrompt)
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

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
	testwait.Until(t, func() bool { return !session.Live() })
	if _, err := registry.Reconnect(context.Background(), session.ID()); err != nil {
		t.Fatal(err)
	}
	spy.at(1).feed(nextShellPrompt)
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

	requireInOrder(t, snapshotOf(session), mouseReportingProgram, reset, nextShellPrompt)
}

// 代替画面を使っていたプログラム（tmux、vimなど）の出力である。
const fullScreenProgram = "\x1b[?1049h\x1b[?1003h"

func TestTheReasonAConnectionWasLostIsWrittenAfterTheModesAreReset(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &openSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	const reason = "\r\nssh: connection lost\r\n"
	spy.at(0).feed(fullScreenProgram)
	lost := transportLost
	lost.Notice = reason
	spy.at(0).exit(lost)
	testwait.Until(t, func() bool { return spy.count() >= 2 })
	spy.at(1).feed(nextShellPrompt)
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

	// 戻す前に書くと、理由は代替画面に書かれ、戻したときに見えなくなる。
	requireInOrder(t, snapshotOf(session), fullScreenProgram, reset, reason, "SSH接続が切れました", nextShellPrompt)
}

func TestModesAreResetBeforeTheNoticeThatAutomaticReconnectStopped(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
		ReconnectError: func(error) (bool, string) { return false, "authentication_rejected" },
	})
	if err != nil {
		t.Fatal(err)
	}
	spy.at(0).finishOpen(nil)
	testwait.Until(t, func() bool { return session.View().State == terminal.StateConnected })

	spy.at(0).feed(mouseReportingProgram)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return spy.count() == 2 })
	const rejection = "[sshc] 認証が拒否されました。"
	spy.at(1).feed(rejection)
	spy.at(1).finishOpen(errors.New("unable to authenticate"))
	spy.at(1).exit(terminal.ExitInfo{Code: 255})
	testwait.Until(t, func() bool { return !session.Live() })

	requireInOrder(t, snapshotOf(session),
		mouseReportingProgram, reset, "SSH接続が切れました", rejection, reset, "自動再接続を停止しました")
}

// 開き直せない試みが続いて自動再接続の上限に達したときも、開き直せなかった理由と
// 上限に達した知らせは、戻したあとの通常の画面へ出る。
func TestModesAreResetBeforeTheNoticesThatReopeningFailedAndTheLimitWasReached(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &openSpy{failUpTo: 1 + terminal.MaxReconnects}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).feed(fullScreenProgram)
	spy.at(0).exit(transportLost)
	testwait.Until(t, func() bool { return !session.Live() })

	requireInOrder(t, snapshotOf(session),
		fullScreenProgram, reset, "SSH接続が切れました", context.DeadlineExceeded.Error(), "再接続できる回数の上限に達しました")
}

// 再接続を待っているあいだに止めたときも、止めた知らせは戻したあとの通常の画面へ出る。
func TestModesAreResetBeforeTheNoticeThatReconnectingWasStoppedWhileWaiting(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &openSpy{}
	registry, _ := newRegistry(terminal.DefaultLimits())
	registry.ReconnectDelay = func(int) time.Duration { return time.Hour }
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	spy.at(0).feed(fullScreenProgram)
	spy.at(0).exit(transportLost)
	// 待ちの知らせが出てから止める。状態は知らせより先に変わるので、状態では待たない。
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte("再接続します")) })
	if err := registry.StopReconnecting(context.Background(), session.ID()); err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return !session.Live() })

	requireInOrder(t, snapshotOf(session), fullScreenProgram, reset, "SSH接続が切れました", "再接続を停止しました")
}

// 最初の接続が失敗したときは自動再接続しないが、終わったプロセスのあとには戻す列を
// 足す。手動の再接続で始まるシェルは、そのあとに出る。
func TestModesAreResetAfterAFirstConnectionThatFailedWithoutReconnecting(t *testing.T) {
	reset := leftoverModeResetFixture(t)
	spy := &readyOpenSpy{}
	registry, _ := newFastRegistry()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindSSH, Alias: "gateway", Title: "gateway", Open: spy.open,
	})
	if err != nil {
		t.Fatal(err)
	}

	const refusal = "[sshc] 接続が拒否されました。"
	spy.at(0).feed(refusal)
	spy.at(0).finishOpen(errors.New("connection refused"))
	spy.at(0).exit(terminal.ExitInfo{Code: 255})
	testwait.Until(t, func() bool { return !session.Live() })
	if _, err := registry.Reconnect(context.Background(), session.ID()); err != nil {
		t.Fatal(err)
	}
	testwait.Until(t, func() bool { return spy.count() == 2 })
	spy.at(1).finishOpen(nil)
	spy.at(1).feed(nextShellPrompt)
	testwait.Until(t, func() bool { return bytes.Contains(snapshotOf(session), []byte(nextShellPrompt)) })

	requireInOrder(t, snapshotOf(session), refusal, reset, nextShellPrompt)
}
