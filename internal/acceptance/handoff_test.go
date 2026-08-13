package acceptance_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// 実行中のアプリケーションは handoff を残し、それを読むのが
// `sshc <alias>` である。これはそのコマンドと同じリクエストを駆動する。
func TestTheHandoffLetsTheCommandLineAskForOneConnection(t *testing.T) {
	f := newFixture(t)

	stateDir := filepath.Join(f.root, "sshc")
	found, err := handoff.Read(stateDir)
	if err != nil {
		t.Fatalf("the running application left no handoff: %v", err)
	}
	info, err := os.Stat(filepath.Join(stateDir, handoff.FileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("handoff mode = %v, %v", info, err)
	}
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
	// このフィクスチャには保存済みパスフレーズが無い。**答えは空でよい**
	// ——コマンドラインは端末で尋ねられる。
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

// **窓を閉じることと、常駐を終わらせることは別の意思である。**
//
// 終了を頼めるのは handoff の秘密を持つ者——つまりこのバイナリ自身——だけで
// ある。画面から常駐を止める道は用意していない。
func TestOnlyTheHandoffSecretCanStopTheEngine(t *testing.T) {
	f := newFixture(t)

	// セッションを持つ画面からは止められない。あそこは cookie と CSRF を
	// 持っているが、handoff の秘密は持たない。
	refused := f.do(http.MethodPost, httpserver.StopPath, []byte(`{}`))
	status := refused.StatusCode
	readBody(t, refused)
	if status != http.StatusForbidden {
		t.Fatalf("a session-authenticated request stopped the engine with %d", status)
	}

	// 秘密なしの匿名も同じである。
	anonymous := f.doAnonymous(http.MethodPost, httpserver.StopPath, []byte(`{}`))
	defer func() { _ = anonymous.Body.Close() }()
	if anonymous.StatusCode != http.StatusForbidden {
		t.Fatalf("an anonymous request stopped the engine with %d", anonymous.StatusCode)
	}

	// 正のコントロール: 秘密を持つ者は止められる。**これが無いと、上の 2 つは
	// 「この経路が誰にも効かない」ことしか言っていない。**
	found, err := handoff.Read(filepath.Join(f.root, "sshc"))
	if err != nil {
		t.Fatal(err)
	}
	accepted := f.doAnonymous(http.MethodPost, httpserver.StopPath, []byte(`{}`),
		func(request *http.Request) {
			request.Header.Set(handoff.HeaderName, found.Secret)
			request.Header.Set("Content-Type", "application/json")
		})
	defer func() { _ = accepted.Body.Close() }()
	if accepted.StatusCode != http.StatusAccepted {
		t.Fatalf("the handoff secret was refused with %d", accepted.StatusCode)
	}
}
