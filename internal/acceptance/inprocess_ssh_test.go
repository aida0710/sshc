package acceptance_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestOpeningAnSSHSessionStartsNoProcess は、B2 の中心的な主張である。
//
// 対話セッションはこのプロセスの中で SSH を話す。外部の ssh も、PTY を確保する
// 継ぎ目も通らない。**ここが緑でなくなったら、権威が外に戻っている。**
func TestOpeningAnSSHSessionStartsNoProcess(t *testing.T) {
	f := newFixture(t)
	f.runner.reset()
	f.terminal.reset()

	response := f.do(http.MethodPost, "/api/v1/terminal/sessions",
		mustJSON(t, map[string]any{"kind": "ssh", "alias": "bastion"}))
	status := response.StatusCode
	body := readBody(t, response)
	if status != http.StatusCreated {
		t.Fatalf("open an ssh session = %d: %s", status, body)
	}

	var opened struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	// 開いたものは閉じる。閉じれば、まだ握手の途中の接続もそこで止まる。
	defer readBody(t, f.do(http.MethodDelete, "/api/v1/terminal/sessions/"+opened.Session.ID, nil))

	if commands := f.runner.recorded(); len(commands) != 0 {
		t.Fatalf("an ssh session started %#v", commands)
	}
	if launched := f.terminal.launched(); len(launched) != 0 {
		t.Fatalf("an ssh session went through the PTY seam: %#v", launched)
	}
}

// 設定が解決できない alias では、セッションそのものを作らない。
//
// 設定の問題は接続画面が既に表示できる。端末に理由を書く必要が無いばかりか、
// 書けば「接続を試みた」という嘘になる。
func TestAnUnresolvableAliasOpensNoSession(t *testing.T) {
	f := newFixture(t)
	mustWrite(t, f.root+"/config", []byte(""+
		"Host refused\n"+
		"\tHostName 203.0.113.10\n"+
		"\tProxyCommand /usr/bin/nc %h %p\n"), 0o600)
	f.runner.reset()

	response := f.do(http.MethodPost, "/api/v1/terminal/sessions",
		mustJSON(t, map[string]any{"kind": "ssh", "alias": "refused"}))
	status := response.StatusCode
	body := readBody(t, response)
	if status < 400 || status >= 500 {
		t.Fatalf("a ProxyCommand alias = %d: %s", status, body)
	}
	if commands := f.runner.recorded(); len(commands) != 0 {
		t.Fatalf("a refused alias still started %#v", commands)
	}

	listing := f.do(http.MethodGet, "/api/v1/terminal/sessions", nil)
	sessions := readBody(t, listing)
	if sessions != "" && sessionCount(t, sessions) != 0 {
		t.Fatalf("a refused alias left a session behind: %s", sessions)
	}
}

func sessionCount(t *testing.T, body string) int {
	t.Helper()
	var listing struct {
		Sessions []json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(body), &listing); err != nil {
		t.Fatal(err)
	}
	return len(listing.Sessions)
}
