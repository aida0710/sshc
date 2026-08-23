package main

import (
	"bytes"
	"strings"
	"testing"

	"sshc/internal/handoff"
)

// 旗は保存された設定より強い。その場で決めたユーザーが居るのに書いてある方を
// 使えば、打った番号がどこにも効かない。
func TestEngineFlagsAreReadIntoTheInvocation(t *testing.T) {
	for _, probe := range []struct {
		args    []string
		port    int
		replace bool
		invalid bool
	}{
		{args: nil},
		{args: []string{"--port", "34567"}, port: 34567},
		{args: []string{"--replace"}, replace: true},
		{args: []string{"--replace", "--port", "1024"}, port: 1024, replace: true},
		// 範囲もここで断る。通すと、断るのは bind の失敗になり、打ったユーザーには
		// 「使えない番号」と「埋まっている番号」が同じに見える。
		{args: []string{"--port", "80"}, invalid: true},
		{args: []string{"--port", "65536"}, invalid: true},
		{args: []string{"--port", "abc"}, invalid: true},
		{args: []string{"--port"}, invalid: true},
		{args: []string{"--nope"}, invalid: true},
	} {
		called, err := parseInvocation(append([]string{"sshc", "engine"}, probe.args...))
		if probe.invalid {
			if err == nil && called.Kind != invocationInvalid {
				t.Errorf("%v was accepted, want it refused", probe.args)
			}
			continue
		}
		if err != nil || called.Kind != invocationEngine {
			t.Errorf("%v = %v, %v; want an engine invocation", probe.args, called, err)
			continue
		}
		if called.Port != probe.port || called.Replace != probe.replace {
			t.Errorf("%v gave port=%d replace=%v, want %d/%v",
				probe.args, called.Port, called.Replace, probe.port, probe.replace)
		}
	}
}

// 対話入力できない環境では置換確認を行わない。
func TestReplacingIsNotOfferedWhereNobodyCanAnswer(t *testing.T) {
	found := handoff.Handoff{PID: 4242, URL: "http://127.0.0.1:34567"}
	var out bytes.Buffer

	// 端末ではない stdin。結果は No で、問いも出ない。
	if askToReplace(found, 0, false, strings.NewReader("y\n"), &out) {
		t.Error("it asked a reader that is not a terminal")
	}
	if out.Len() != 0 {
		t.Errorf("it printed %q to a place nobody is reading", out.String())
	}
}

// 旗を書いたユーザーには従う。端末かどうかは関係ない。先に結果を決めてある。
func TestReplacingObeysTheFlagWithoutAsking(t *testing.T) {
	var out bytes.Buffer
	if !askToReplace(handoff.Handoff{PID: 1}, 3, true, strings.NewReader(""), &out) {
		t.Error("--replace was not obeyed")
	}
	if out.Len() != 0 {
		t.Errorf("it asked anyway: %q", out.String())
	}
}
