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

// **ウィンドウを閉じることと、常駐を終わらせることは別の意思である。**
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

// TestTheCommandLineCanTellThisEngineIsAlive は、**本物のサーバーに対して**
// 生死の判定を駆動する。
//
// この検査が置かれたのは、偽物を相手にした単体テストが緑のまま製品が壊れて
// いたからである。cmd/sshc の fixture は /api/v1/health に 200 を返す
// httptest.Server だったが、**本物の /api/ は Sec-Fetch-Site: same-origin を
// 要求する**ので、ブラウザでないものは通れなかった。偽物はミドルウェアを
// 模していなかった。
func TestTheCommandLineCanTellThisEngineIsAlive(t *testing.T) {
	f := newFixture(t)
	found, err := handoff.Read(filepath.Join(f.root, "sshc"))
	if err != nil {
		t.Fatal(err)
	}

	// 秘密を持つ者には答える。
	alive := f.doAnonymous(http.MethodPost, httpserver.HealthPath, []byte(`{}`),
		func(request *http.Request) {
			request.Header.Set(handoff.HeaderName, found.Secret)
			request.Header.Set("Content-Type", "application/json")
		})
	defer func() { _ = alive.Body.Close() }()
	if alive.StatusCode != http.StatusNoContent {
		t.Fatalf("the running engine answered %d, so the shell would start a second one", alive.StatusCode)
	}

	// 秘密を持たない者には答えない。**「何かが答えた」ではなく「我々の
	// エンジンが答えた」と言えなければ、ポートの再利用に繋いでしまう。**
	refused := f.doAnonymous(http.MethodPost, httpserver.HealthPath, []byte(`{}`))
	defer func() { _ = refused.Body.Close() }()
	if refused.StatusCode != http.StatusForbidden {
		t.Fatalf("an anonymous probe was answered with %d", refused.StatusCode)
	}

	// **この経路を分けた理由をここに残す。** 外殻はブラウザではないので
	// Sec-Fetch-Site を持たず、/api/ の下は何ひとつ通れない——最初はそこへ
	// 問いに行き、製品が壊れていた。
	viaAPI := f.doAnonymous(http.MethodGet, "/api/v1/health", nil)
	defer func() { _ = viaAPI.Body.Close() }()
	// 断り方（401 か 403 か）は問わない。**通れないことが要点**であり、
	// どちらで断るかはミドルウェアの順序の話である。
	if viaAPI.StatusCode < 400 {
		t.Fatalf("a client without browser headers reached /api/v1/health with %d; "+
			"the separate /cli route would no longer be needed, and this test would be stale",
			viaAPI.StatusCode)
	}
}
