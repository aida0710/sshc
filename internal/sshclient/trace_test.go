package sshclient

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"sshc/internal/connectionlog"
)

// 既定は無言である。毎回この量が流れると、シェルの最初の一画面が押し流される。
func TestATracerSaysNothingUntilItIsAsked(t *testing.T) {
	var out bytes.Buffer
	trace := newTracer(connectionlog.Notice, &out)
	trace.say(connectionlog.Brief, "繋ぎます")
	trace.say(connectionlog.Detailed, "鍵を試します")
	trace.say(connectionlog.Full, "算法は %s", "x")
	if out.Len() != 0 {
		t.Errorf("quiet の tracer が書いた: %q", out.String())
	}
}

// 求められた深さまでを言い、それより深い話はしない。
func TestATracerStopsAtTheDepthItWasGiven(t *testing.T) {
	for _, probe := range []struct {
		level connectionlog.Level
		want  []connectionlog.Level
		skip  []connectionlog.Level
	}{
		{level: connectionlog.Brief, want: []connectionlog.Level{connectionlog.Brief}, skip: []connectionlog.Level{connectionlog.Detailed, connectionlog.Full}},
		{level: connectionlog.Detailed, want: []connectionlog.Level{connectionlog.Brief, connectionlog.Detailed}, skip: []connectionlog.Level{connectionlog.Full}},
		{level: connectionlog.Full, want: []connectionlog.Level{connectionlog.Brief, connectionlog.Detailed, connectionlog.Full}},
	} {
		var out bytes.Buffer
		trace := newTracer(probe.level, &out)
		for _, level := range []connectionlog.Level{connectionlog.Brief, connectionlog.Detailed, connectionlog.Full} {
			trace.say(level, "level-%d", int(level))
		}
		for _, level := range probe.want {
			if !strings.Contains(out.String(), "level-"+string(rune('0'+int(level)))) {
				t.Errorf("level %d で level-%d が出ていない: %q", probe.level, level, out.String())
			}
		}
		for _, level := range probe.skip {
			if strings.Contains(out.String(), "level-"+string(rune('0'+int(level)))) {
				t.Errorf("level %d で level-%d まで出た: %q", probe.level, level, out.String())
			}
		}
	}
}

// 端末は行末にCRLFを要るが、行頭にも置くと連続する診断の間に空行が生まれる。
// PTYはここを通っていないので、必要な改行を出力側が一つだけ置く。
func TestEveryTracedLineEndsTheWayATerminalNeeds(t *testing.T) {
	var out bytes.Buffer
	trace := newTracer(connectionlog.Brief, &out)
	trace.say(connectionlog.Brief, "繋ぎます")
	trace.say(connectionlog.Brief, "接続完了")
	written := out.String()
	want := "[sshc][debug1] 繋ぎます\r\n[sshc][debug1] 接続完了\r\n"
	if written != want {
		t.Errorf("written = %q, want %q", written, want)
	}
	if strings.Contains(strings.ReplaceAll(written, "\r\n", ""), "\n") {
		t.Errorf("written = %q, want no bare newline", written)
	}
}

// nil の tracer でも落ちない。途中経過を出さない道（`--non-interactive` や到達確認）は
// tracer を持たないまま同じ関数を通る。
func TestANilTracerIsSafeToUse(t *testing.T) {
	var trace *tracer
	trace.say(connectionlog.Brief, "落ちない")
	if trace.enabled(connectionlog.Brief) {
		t.Error("nil tracer reported itself as writable")
	}
	if trace.now().IsZero() {
		t.Error("nil tracer did not use the supplied clock")
	}
	if trace.since(time.Now().Add(-time.Second)) <= 0 {
		t.Error("nil の tracer が経過を測れなかった")
	}
}

// 行の頭には、その行を出した深さが付く。OpenSSH の debug1:／debug2:／debug3:
// と同じで、読む側はどの設定で出た行かを行だけで判別できる。設定に関係なく
// 出る行（ProxyCommand の実行）には深さの印が無い。
func TestEachTracedLineCarriesTheDepthThatProducedIt(t *testing.T) {
	var out bytes.Buffer
	trace := newTracer(connectionlog.Full, &out)
	trace.say(connectionlog.Brief, "繋ぎます")
	trace.say(connectionlog.Detailed, "鍵を試します")
	trace.say(connectionlog.Full, "算法は x")
	trace.announce("ProxyCommandを実行します")
	want := "[sshc][debug1] 繋ぎます\r\n" +
		"[sshc][debug2] 鍵を試します\r\n" +
		"[sshc][debug3] 算法は x\r\n" +
		"[sshc] ProxyCommandを実行します\r\n"
	if out.String() != want {
		t.Errorf("written = %q, want %q", out.String(), want)
	}
}

// debug2 では、ホップで使う設定と経路の種類を言う。
func TestTheHopSettingsAreDescribedAtDetailed(t *testing.T) {
	var out strings.Builder
	trace := newTracer(connectionlog.Detailed, &out)

	describeHop(trace, Target{HostName: "10.0.0.5", Port: "22", User: "aida", Identities: []string{"/home/a/.ssh/id_ed25519"}, VPN: "lab"})

	for _, want := range []string{"HostName 10.0.0.5、Port 22、User aida、IdentityFile /home/a/.ssh/id_ed25519", "経路：VPNプロファイル「lab」"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("out = %q", out.String())
		}
	}
}

// 利用者向けの文に置き換えた失敗も、詳細と元の失敗を debug2 に残す。
func TestAnExplainedFailureKeepsItsDetailsAtDetailed(t *testing.T) {
	var out strings.Builder
	trace := newTracer(connectionlog.Detailed, &out)

	explainFailure(trace, &ExplainedError{Sentence: "作れませんでした。", Details: []string{"ERROR: failed"}, Err: errors.New("exit status 1")})

	for _, want := range []string{"[sshc][debug2]   ERROR: failed", "[sshc][debug2] 失敗の詳細：exit status 1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("out = %q", out.String())
		}
	}
}

// VPN の経路の知らせ（connectionlog.Notice）は、既定の無言でも [sshc] の行として出す。
// 深さの印は付けない。
func TestAConnectionNoticeIsShownEvenWhenQuiet(t *testing.T) {
	var out bytes.Buffer
	trace := newTracer(connectionlog.Notice, &out)
	ctx := trace.withLog(context.Background())

	connectionlog.Say(ctx, connectionlog.Notice, "VPNのコンテナイメージを作成しています。")
	connectionlog.Say(ctx, connectionlog.Brief, "書かない")

	if got := out.String(); got != "[sshc] VPNのコンテナイメージを作成しています。\r\n" {
		t.Fatalf("out = %q", got)
	}
}

// 複数行の文（docker build の出力など）は、行ごとに印を付けて CRLF で書く。LF の
// ままだと端末で行頭へ戻らず、階段状に崩れる。
func TestAMultiLineMessageIsWrittenLineByLine(t *testing.T) {
	var out bytes.Buffer
	trace := newTracer(connectionlog.Detailed, &out)

	trace.say(connectionlog.Detailed, "失敗の詳細：%s", "一行目\n二行目\r\n")

	if got := out.String(); got != "[sshc][debug2] 失敗の詳細：一行目\r\n[sshc][debug2] 二行目\r\n" {
		t.Fatalf("out = %q", got)
	}
}

// 失敗の行は、どの段階で失敗したかだけを言う。理由は最後の sshc: の行が言う。
func TestTheFailureLineLeavesTheReasonToTheFinalLine(t *testing.T) {
	explained := &ExplainedError{Sentence: "Dockerが起動していません。", Err: errors.New("docker is not running")}

	if got := connectionFailureMessage("接続", explained); got != "接続に失敗しました。" {
		t.Fatalf("connectionFailureMessage = %q", got)
	}
}

// 端末へ書く文の改行は CRLF にする。すでに CRLF の改行は二重にしない。
func TestTerminalNewlinesReturnToTheStartOfTheLine(t *testing.T) {
	if got := terminalNewlines("a\nb\r\nc"); got != "a\r\nb\r\nc" {
		t.Fatalf("terminalNewlines = %q", got)
	}
}
