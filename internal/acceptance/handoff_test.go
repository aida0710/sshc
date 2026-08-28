package acceptance_test

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// 実行中のアプリケーションは handoff を残し、それを読むのが
// `sshc ssh <alias>` である。これはそのコマンドと同じリクエストを駆動する。
func TestTheHandoffLetsTheCommandLineAskForOneConnection(t *testing.T) {
	f := newFixture(t)

	stateDir := filepath.Join(f.root, "sshc")
	found, err := handoff.Read(stateDir)
	if err != nil {
		t.Fatalf("the running application left no handoff: %v", err)
	}
	assertHandoffIsPrivate(t, filepath.Join(stateDir, handoff.FileName))
	if found.URL != f.baseURL {
		t.Errorf("the handoff names %q, the server is at %q", found.URL, f.baseURL)
	}

	// コマンドラインには session も CSRF トークンもなく、どちらも要らない。
	response := f.doAnonymous(http.MethodPost, httpserver.ConnectPath,
		[]byte(`{"alias":"bastion"}`), func(request *http.Request) {
			request.Header.Set(handoff.HeaderName, found.Secret)
		})
	status := response.StatusCode
	body := readBody(t, response)
	if status != http.StatusOK {
		t.Fatalf("connect = %d (%s)", status, body)
	}
	var answer struct {
		Alias      string   `json:"alias"`
		KeyPath    string   `json:"keyPath"`
		Passphrase string   `json:"passphrase"`
		Warnings   []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Alias != "bastion" {
		t.Errorf("answer = %+v", answer)
	}
	// このフィクスチャには保存済みパスフレーズが無い。結果は空でよい
	//コマンドラインは端末で尋ねられる。
	if answer.Passphrase != "" {
		t.Errorf("the answer carried a passphrase nobody stored: %+v", answer)
	}

	// secret がなければ、何も語らない。
	refused := f.doAnonymous(http.MethodPost, httpserver.ConnectPath, []byte(`{"alias":"bastion"}`))
	defer func() { _ = refused.Body.Close() }()
	if refused.StatusCode != http.StatusForbidden {
		t.Errorf("connect without the secret = %d, want 403", refused.StatusCode)
	}
}
