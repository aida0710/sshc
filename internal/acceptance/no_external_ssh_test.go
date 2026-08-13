package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"sshc/internal/session"
)

// TestNoConnectionRouteStartsAnExternalProgram は、B3 の完成条件である。
//
// 接続に関わる経路——設定の解決、実効値、到達性、認証、ホスト鍵の取得、
// 公開鍵のリモート登録、対話ターミナル——のどれも、外部プログラムを起こさない。
// **ここが緑でなくなったら、OpenSSH のプログラムが戻ってきている。**
//
// この検査が置かれたのは askpass の一式を消す直前である。消せることの証明を
// 先に置き、そのうえで消す。
func TestNoConnectionRouteStartsAnExternalProgram(t *testing.T) {
	f := newFixture(t)
	f.runner.reset()
	f.scanner.reset()

	// 確認を要する経路には正しい token を渡す。**拒否されたから起動しなかった、
	// では何も証明できない。**
	requests := []struct {
		path  string
		body  map[string]any
		token func() string
	}{
		{"/api/v1/diagnostics/config", map[string]any{}, nil},
		{"/api/v1/diagnostics/effective", map[string]any{"alias": "bastion"}, nil},
		{"/api/v1/diagnostics/reachability", map[string]any{"alias": "bastion"},
			func() string { return f.actionToken(t, session.ActionReachability, "bastion") }},
		{"/api/v1/diagnostics/authentication",
			map[string]any{"alias": "bastion", "acknowledgeExecutable": true},
			func() string { return f.actionToken(t, session.ActionAuthentication, "bastion") }},
		{"/api/v1/known-hosts/scan", map[string]any{"host": "203.0.113.10", "port": 22},
			func() string { return f.actionToken(t, session.ActionKnownHostsScan, "203.0.113.10") }},
		{"/api/v1/remote-keys/register", map[string]any{
			"alias": "bastion", "keyPath": "id_ed25519.pub",
			"publicKey":             strings.TrimSpace(string(f.read("id_ed25519.pub"))),
			"acknowledgeExecutable": true,
		}, func() string { return f.remoteKeyPlanToken(t, "bastion") }},
		{"/api/v1/terminal/sessions", map[string]any{"kind": "ssh", "alias": "bastion"}, nil},
	}

	reached := 0
	for _, request := range requests {
		token := ""
		if request.token != nil {
			token = request.token()
		}
		response := f.do(http.MethodPost, request.path, mustJSON(t, request.body), withAction(token))
		status := response.StatusCode
		body := readBody(t, response)
		if status >= 500 {
			t.Fatalf("%s = %d: %s", request.path, status, body)
		}
		if status < 300 {
			reached++
		}
	}

	// 正のコントロール: 上のいくつかは実際に働かなければならない。全部が
	// 拒否されていたら、「何も起動しなかった」は当たり前の話になる。
	if reached < len(requests)-1 {
		t.Fatalf("only %d of %d routes did anything; the claim below proves little", reached, len(requests))
	}
	if commands := f.runner.recorded(); len(commands) != 0 {
		t.Fatalf("a connection route started %#v", commands)
	}
	if launched := f.terminal.launched(); len(launched) != 0 {
		t.Fatalf("a connection route went through the PTY seam: %#v", launched)
	}
}
