package main

import (
	"context"
	"testing"
	"time"

	"sshc/internal/handoff"
)

// コマンドの全体が alias である。それ以外はフラグか、askpass サブコマンドか、
// アプリケーション自身のいずれかだ。
func TestWhatCountsAsAConnectInvocation(t *testing.T) {
	for _, test := range []struct {
		argv  []string
		alias string
		ok    bool
	}{
		{[]string{"sshc", "tv-recoding"}, "tv-recoding", true},
		{[]string{"sshc", "mdx-aida-serv-1"}, "mdx-aida-serv-1", true},
		// 接続ではなく、アプリケーション。
		{[]string{"sshc"}, "", false},
		{[]string{"sshc", "-open=false"}, "", false},
		{[]string{"sshc", "--open=false"}, "", false},
		// ホストではなくコマンドである語。
		{[]string{"sshc", "open"}, "", false},
		{[]string{"sshc", "list"}, "", false},
		{[]string{"sshc", "connect"}, "", false},
		// 外殻がエンジンの様子を尋ねる語。
		{[]string{"sshc", "status"}, "", false},
		// 二語は alias ではない。このコマンドには存在しないものだ。
		{[]string{"sshc", "connect", "bastion"}, "", false},
	} {
		alias, ok := connectInvocation(test.argv)
		if ok != test.ok || alias != test.alias {
			t.Errorf("connectInvocation(%v) = %q, %v; want %q, %v", test.argv, alias, ok, test.alias, test.ok)
		}
	}
}

// `sshc help` は使い方を出す。
//
// これがないと "help" は裸の語として alias になり、`ssh -- help` として exec される。
// 使い方を求めた人が受け取るのは
// "Could not resolve hostname help" である。-h は flag パッケージが拾うが、
// 打つ言葉として自然なのはこちらであり、`open` と `list` を予約したのと同じ理由で
// 予約する。
func TestHelpIsASubcommandAndNotAnAlias(t *testing.T) {
	if alias, ok := connectInvocation([]string{"sshc", HelpSubcommand}); ok {
		t.Fatalf("help was read as the alias %q", alias)
	}
	for _, word := range []string{"help", "-h", "--help"} {
		if !helpInvocation([]string{"sshc", word}) {
			t.Errorf("helpInvocation(%q) = false, want true", word)
		}
	}
	// 引数を伴えば使い方ではない。`sshc help me` はホスト "help" への接続でもない。
	if helpInvocation([]string{"sshc", "help", "me"}) {
		t.Error("help with an argument was read as a request for usage")
	}
	if helpInvocation([]string{"sshc", "bastion"}) {
		t.Error("an ordinary alias was read as a request for usage")
	}
}

// **ロックを取れることと、入口が読めることは同時ではない。** 勝った方は
// listener を上げてから handoff を書くので、負けた方がその隙に読むと
// 「sshc: not running」になる。ほぼ同時に打った 2 つのうち片方だけが、
// 理由の無い失敗を受け取ることになっていた。
func TestWaitForHandoffWaitsForTheWinnerToWrite(t *testing.T) {
	stateDir := t.TempDir()
	go func() {
		time.Sleep(150 * time.Millisecond)
		_, _ = handoff.Write(stateDir, "http://127.0.0.1:1", "the secret")
	}()

	waitForHandoff(context.Background(), stateDir)
	if _, err := handoff.Read(stateDir); err != nil {
		t.Fatalf("waitForHandoff returned before the handoff was readable: %v", err)
	}
}

// **待つのは待てるあいだだけである。** Ctrl-C を押した人を、居ないかもしれない
// エンジンのために 4 秒立たせない。
func TestWaitForHandoffGivesUpWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		waitForHandoff(ctx, t.TempDir())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForHandoff kept waiting after the context was done")
	}
}
