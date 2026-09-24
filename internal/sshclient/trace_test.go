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
	trace := newTracer(Quiet, &out)
	trace.say(Brief, "繋ぎます")
	trace.say(Detailed, "鍵を試します")
	trace.say(Full, "算法は %s", "x")
	if out.Len() != 0 {
		t.Errorf("quiet の tracer が書いた: %q", out.String())
	}
}

// 求められた深さまでを言い、それより深い話はしない。
func TestATracerStopsAtTheDepthItWasGiven(t *testing.T) {
	for _, probe := range []struct {
		level Verbosity
		want  []Verbosity
		skip  []Verbosity
	}{
		{level: Brief, want: []Verbosity{Brief}, skip: []Verbosity{Detailed, Full}},
		{level: Detailed, want: []Verbosity{Brief, Detailed}, skip: []Verbosity{Full}},
		{level: Full, want: []Verbosity{Brief, Detailed, Full}},
	} {
		var out bytes.Buffer
		trace := newTracer(probe.level, &out)
		for _, level := range []Verbosity{Brief, Detailed, Full} {
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
	trace := newTracer(Brief, &out)
	trace.say(Brief, "繋ぎます")
	trace.say(Brief, "接続完了")
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
	trace.say(Brief, "落ちない")
	if trace.enabled(Brief) {
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
	trace := newTracer(Full, &out)
	trace.say(Brief, "繋ぎます")
	trace.say(Detailed, "鍵を試します")
	trace.say(Full, "算法は x")
	trace.announce("ProxyCommand を実行します")
	want := "[sshc][debug1] 繋ぎます\r\n" +
		"[sshc][debug2] 鍵を試します\r\n" +
		"[sshc][debug3] 算法は x\r\n" +
		"[sshc] ProxyCommand を実行します\r\n"
	if out.String() != want {
		t.Errorf("written = %q, want %q", out.String(), want)
	}
}

// debug2 では、ホップで使う設定と経路の種類を言う。
func TestTheHopSettingsAreDescribedAtDetailed(t *testing.T) {
	var out strings.Builder
	trace := newTracer(Detailed, &out)

	describeHop(trace, Target{HostName: "10.0.0.5", Port: "22", User: "aida", Identities: []string{"/home/a/.ssh/id_ed25519"}, VPN: "lab"})

	for _, want := range []string{"HostName 10.0.0.5、Port 22、User aida、IdentityFile /home/a/.ssh/id_ed25519", "経路：VPNプロファイル lab"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("out = %q", out.String())
		}
	}
}

// 利用者向けの文に置き換えた失敗も、詳細と元の失敗を debug2 に残す。
func TestAnExplainedFailureKeepsItsDetailsAtDetailed(t *testing.T) {
	var out strings.Builder
	trace := newTracer(Detailed, &out)

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
	trace := newTracer(Quiet, &out)
	ctx := trace.withLog(context.Background())

	connectionlog.Say(ctx, connectionlog.Notice, "VPNのコンテナイメージを作成しています。")
	connectionlog.Say(ctx, connectionlog.Brief, "書かない")

	if got := out.String(); got != "[sshc] VPNのコンテナイメージを作成しています。\r\n" {
		t.Fatalf("out = %q", got)
	}
}
