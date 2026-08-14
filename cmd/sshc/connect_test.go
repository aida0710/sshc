package main

import (
	"testing"
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
		{[]string{"sshc", "service"}, "", false},
		// 常駐そのものを起こしたり止めたりする語。
		{[]string{"sshc", "engine"}, "", false},
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
