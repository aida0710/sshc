package acceptance_test

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"sshc/internal/app"
	"sshc/internal/handoff"
)

// nil は、機能が無いという結果である。
//
// Android には ssh-keygen も ssh-agent も居ないので、それを探すツールを持つこと
// 自体が誤りになる。置き換えられない更新を提示するのも同じである。だから
// mobile/dependencies.go は、それらを nil のまま engine へ渡す。
//
// 受け側がそれを扱い損ねると、落ちるのは実機だけである。開発機の go test は
// 常にツールが在る姿で走るので、nil の枝を一度も通らない。
//
// ここは、mobile が nil にするものを数えたうえで、その姿の engine が
// 実際に返すことを見る。数えるのは、nil が増えたときにこの検査が
// 暗黙に古いままにならないためである。

// nilledByMobile は、mobile/dependencies.go が nil を渡している依存の名前を返す。
func nilledByMobile(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "mobile", "dependencies.go"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^\s*([A-Z][A-Za-z]*):\s*nil,`).FindAllStringSubmatch(string(body), -1)
	names := make([]string, 0, len(found))
	for _, match := range found {
		names = append(names, match[1])
	}
	return names
}

func TestTheEngineAnswersInTheShapeMobileGivesIt(t *testing.T) {
	// この検査が扱いを知っているもの。mobile が新しく nil を渡し始めたら、
	// ここに足すまで赤くなる。
	handled := map[string]bool{
		"Toolchain": true,
		"KeyAgent":  true,
		"Updates":   true,
	}
	for _, name := range nilledByMobile(t) {
		if !handled[name] {
			t.Errorf("mobile が %s を nil で渡すようになったが、この検査はその姿を確かめていない", name)
		}
	}
	if len(nilledByMobile(t)) != len(handled) {
		t.Errorf("mobile が nil にするのは %v、この検査が知っているのは %d 件",
			nilledByMobile(t), len(handled))
	}

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}

	// mobile/dependencies.go と同じ姿。ツールは一つも渡さない。
	server, bootstrap, err := app.Build(app.Dependencies{
		Home:   home,
		Owner:  handoff.OwnerEngine,
		PID:    4242,
		Random: rand.Reader,
		Listen: net.Listen,
		UI: fstest.MapFS{"index.html": {
			Data: []byte(`<!doctype html><title>fixture</title><div id="root"></div>`),
		}},
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Toolchain: nil,
		KeyAgent:  nil,
		Updates:   nil,
		Lookup:    func(string) (string, bool) { return "", false },
		Environ:   func() []string { return []string{"HOME=" + home} },
	}, "mobile-shape")
	if err != nil {
		t.Fatalf("app.Build with the mobile shape = %v", err)
	}

	served := make(chan error, 1)
	go func() { served <- server.Serve() }()
	t.Cleanup(func() {
		server.BeginStopping()
		server.BeginShutdown()
		_ = server.Wait()
		if err := <-served; err != nil {
			t.Errorf("Serve() = %v", err)
		}
	})

	// API は session の背後にある。ネイティブ層がやるのと同じ交換をここでも通す
	//アクセス URLを開いた WebView が最初にすることである。
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}

	exchange, err := http.NewRequest(http.MethodPost, server.URL()+"/api/v1/session/bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	exchange.Header.Set("X-SSHC-Bootstrap", bootstrap)
	// 同一生成元であることを言う。本物の WebView が送るものと同じ。
	exchange.Header.Set("Sec-Fetch-Site", "same-origin")
	exchange.Header.Set("Origin", server.URL())
	opened, err := client.Do(exchange)
	if err != nil {
		t.Fatalf("bootstrap = %v", err)
	}
	if opened.StatusCode != http.StatusOK {
		_ = opened.Body.Close()
		t.Fatalf("bootstrap = %d, want 200", opened.StatusCode)
	}
	var session struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(opened.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = opened.Body.Close()

	// token は read にも伴う。cookie はポートに scope されないが token はされる。
	get := func(path string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(http.MethodGet, server.URL()+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.Header.Set("X-SSHC-CSRF", session.CsrfToken)
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("GET %s = %v", path, err)
		}
		return response
	}

	// すべての route が master password の背後にある。金庫を作るのは、
	// パスワードの検査ではなくアプリケーション起動の一部である。
	initialise, err := http.NewRequest(http.MethodPost, server.URL()+"/api/v1/passwords/initialise",
		strings.NewReader(`{"passphrase":"a fixture master password"}`))
	if err != nil {
		t.Fatal(err)
	}
	initialise.Header.Set("Sec-Fetch-Site", "same-origin")
	initialise.Header.Set("Origin", server.URL())
	initialise.Header.Set("Content-Type", "application/json")
	initialise.Header.Set("X-SSHC-CSRF", session.CsrfToken)
	unlocked, err := client.Do(initialise)
	if err != nil {
		t.Fatalf("initialise = %v", err)
	}
	_ = unlocked.Body.Close()
	if unlocked.StatusCode != http.StatusOK {
		t.Fatalf("initialise the vault = %d, want 200", unlocked.StatusCode)
	}

	// Sync route は mobile でも desktop と同じ service を使う。app.Build は Auto を
	// 配線する一方、起動直後にはまだ最初の巡回が来ていない。その時点でも Web の
	// runtime validator が読める完全な状態を返さなければ、Android だけ Sync 画面が
	// 「状態を読み込めませんでした」になる。
	syncStatus := get("/api/v1/sync")
	defer func() { _ = syncStatus.Body.Close() }()
	if syncStatus.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/sync = %d, want 200", syncStatus.StatusCode)
	}
	var cleanSync struct {
		Configured    bool   `json:"configured"`
		Synced        bool   `json:"synced"`
		Direction     string `json:"direction"`
		KeyConfigured bool   `json:"keyConfigured"`
		Auto          struct {
			Enabled bool   `json:"enabled"`
			Phase   string `json:"phase"`
		} `json:"auto"`
	}
	if err := json.NewDecoder(syncStatus.Body).Decode(&cleanSync); err != nil {
		t.Fatalf("decode clean sync status: %v", err)
	}
	if cleanSync.Configured || cleanSync.Synced || cleanSync.KeyConfigured || cleanSync.Auto.Enabled {
		t.Errorf("a clean mobile engine reported configured sync: %+v", cleanSync)
	}
	if cleanSync.Direction != "both" || cleanSync.Auto.Phase != "idle" {
		t.Errorf("clean mobile sync state = direction %q, auto phase %q; want both, idle",
			cleanSync.Direction, cleanSync.Auto.Phase)
	}

	// 更新: 版だけを結果、「新しいものがある」とは言わない。
	//
	// Checker が nil のとき、ここが落ちれば Android の画面は起動直後に赤くなる。
	response := get("/api/v1/update")
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/update = %d, want 200", response.StatusCode)
	}
	var update struct {
		Current   string  `json:"current"`
		Available bool    `json:"available"`
		Latest    *string `json:"latest"`
	}
	if err := json.NewDecoder(response.Body).Decode(&update); err != nil {
		t.Fatal(err)
	}
	if update.Current != "mobile-shape" {
		t.Errorf("current = %q, want the running version", update.Current)
	}
	if update.Available || update.Latest != nil {
		// 置き換えられない更新を提示しない。Android の APK は、この engine が
		// 入れ替えられるものではない。
		t.Errorf("an engine with no checker offered an update: %+v", update)
	}

	// ツールが無い枝を実際に踏む。
	//
	// Toolchain が nil なら、ハードウェア鍵は一覧に出ない。ssh-keygen が
	// 無い機械で「YubiKey で作れます」と言うのは誤りである。
	algorithms := get("/api/v1/keys/algorithms")
	defer func() { _ = algorithms.Body.Close() }()
	if algorithms.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/keys/algorithms = %d, want 200", algorithms.StatusCode)
	}
	var catalogue struct {
		Variants []struct {
			Algorithm string `json:"algorithm"`
		} `json:"variants"`
	}
	if err := json.NewDecoder(algorithms.Body).Decode(&catalogue); err != nil {
		t.Fatal(err)
	}
	if len(catalogue.Variants) == 0 {
		t.Error("no key algorithm is offered at all; this engine generates keys itself")
	}
	for _, variant := range catalogue.Variants {
		if strings.HasSuffix(variant.Algorithm, "-sk") {
			t.Errorf("an engine with no toolchain offered the hardware key %q", variant.Algorithm)
		}
	}

	// KeyAgent が nil なら、届くエージェントは無いと返す。
	keys := get("/api/v1/keys")
	defer func() { _ = keys.Body.Close() }()
	if keys.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/keys = %d, want 200", keys.StatusCode)
	}
	var inventory struct {
		AgentAvailable bool `json:"agentAvailable"`
	}
	if err := json.NewDecoder(keys.Body).Decode(&inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.AgentAvailable {
		t.Error("an engine with no key agent reported one as reachable")
	}

	// 画面は出る。ツールが無いことは、アプリが開かないことではない。
	page := get("/")
	defer func() { _ = page.Body.Close() }()
	shell, err := io.ReadAll(page.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shell), `<div id="root">`) {
		t.Errorf("the entrance did not return the app shell: %q", shell)
	}
}
