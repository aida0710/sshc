package sshclient

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
	want := "[sshc] 繋ぎます\r\n[sshc] 接続完了\r\n"
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
