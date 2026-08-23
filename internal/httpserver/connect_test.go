package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/handoff"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

func connectEngine(t *testing.T, handlers ConnectHandlers) *echo.Echo {
	t.Helper()
	engine := echo.New()
	registerConnectRoutes(engine, handlers)
	return engine
}

// アカウントのパスワードが、鍵のパスフレーズとして返ることは無い。
//
// 名前空間が別だからである。混ぜれば、ローカルの鍵を開くための秘密が
// リモートへログインパスワードとして送られる。
func TestAnAccountPasswordNeverComesBackAsAKeyPassphrase(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("bastion", "legacy-password"); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("connect = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Passphrases) != 0 {
		t.Fatalf("an account password came back as a key passphrase: %+v", answer)
	}
	// 自分の欄には載る。保存してあるのに使わないなら、保存させる意味が無い。
	if answer.Passwords["bastion"] != "legacy-password" {
		t.Fatalf("the stored account password did not reach the command line: %+v", answer)
	}
}

// 保存済みアカウントパスワードだけを持つ alias は、鍵についての結果を持たない。
// 返るのはパスワードひとつであり、鍵の欄は空のままである。
func TestAnAliasWithOnlyAnAccountPasswordCarriesNoKey(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("password-only", "stored-account-password"); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"password-only"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("connect = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Passphrases) != 0 {
		t.Fatalf("a stored account password produced a key answer: %+v", answer)
	}
	if answer.Passwords["password-only"] != "stored-account-password" {
		t.Fatalf("the stored account password did not reach the command line: %+v", answer)
	}
}

// 連鎖ぶんを返す。ProxyJump の手前に立つホストも、それ自身が alias である。
//
// 同時に、返すのはこの接続に現れるものだけである。保管庫の一覧にはしない。
func TestConnectCarriesThePasswordsOfTheWholeJumpChain(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	for alias, password := range map[string]string{
		"far": "the destination", "edge": "the way in", "elsewhere": "nothing to do with this",
	} {
		if err := vault.Set(alias, password); err != nil {
			t.Fatal(err)
		}
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
		Aliases: func(alias string) []string { return []string{"edge", alias} },
	})

	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"far"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("connect = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}

	if answer.Passwords["far"] != "the destination" || answer.Passwords["edge"] != "the way in" {
		t.Fatalf("the chain did not arrive whole: %+v", answer.Passwords)
	}
	// 尋ねられた接続に現れないものは返さない。
	if _, listed := answer.Passwords["elsewhere"]; listed {
		t.Fatalf("the answer listed the vault: %+v", answer.Passwords)
	}
}

func TestConnectAnswersWithTheKeyPassphraseForTheDirectStoredKey(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.SetCredential(secret.KindKeyPassphrase, "server-key", "saved key phrase"); err != nil {
		t.Fatal(err)
	}
	if err := vault.AssignCredential(secret.KindKeyPassphrase, "id_ed25519_server", "server-key"); err != nil {
		t.Fatal(err)
	}

	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
		WorkspaceKeys: func(alias string) ([]string, error) {
			if alias != "bastion" {
				return nil, nil
			}
			return []string{"id_ed25519_server"}, nil
		},
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("connect = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	// 結果そのものが返る。単回トークンにしていたのは、引き換える相手が
	// 別のプログラムだったからである。要求を出したユーザー本人が受け取るなら、
	// 発行と引き換えを分ける理由が無い。
	if answer.Passphrases["id_ed25519_server"] == "" {
		t.Fatalf("connect answer = %+v, want the stored passphrase under its key path", answer)
	}
}

func TestConnectDoesNotFallBackToAnAccountPasswordWhenKeyResolutionFails(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("bastion", "legacy-password"); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
		WorkspaceKeys: func(string) ([]string, error) {
			return nil, os.ErrPermission
		},
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Passphrases) != 0 {
		t.Fatal("an unreadable key policy fell back to the account-password namespace")
	}
}

// secret がなければ、このエンドポイントは何も語らない。alias が
// 未知であることも、パスワードが保存されていることも、いないことも。
func TestConnectRefusesWithoutTheSecretAndSaysNothingElse(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: "the secret for this run"})

	for _, presented := range []string{"", "the wrong secret", "the secret for this ru"} {
		recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
			map[string]string{handoff.HeaderName: presented})
		if recorder.Code != http.StatusForbidden {
			t.Errorf("with %q = %d, want 403", presented, recorder.Code)
		}
		if recorder.Body.Len() != 0 {
			t.Errorf("with %q the body is %q", presented, recorder.Body.String())
		}
	}
}

// handoff を書けなかったサーバーは、すべてを受け入れるのではなく
// 何も受け入れない。
func TestConnectWithNoSecretConfiguredRefusesEveryone(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: ""})
	if recorder.Code != http.StatusForbidden {
		t.Errorf("= %d, want 403", recorder.Code)
	}
}

// vault がない場合の結果はトークンなしの接続であり、それは正常な接続である。
// OpenSSH 自身がパスワードを尋ねる。
func TestConnectAnswersWithNothingWhenNothingIsStored(t *testing.T) {
	const secret = "the secret for this run"
	engine := connectEngine(t, ConnectHandlers{
		Secret:   secret,
		Warnings: func(string) []string { return []string{"ProxyCommand runs on connect"} },
	})

	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: secret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("= %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Passphrases) != 0 {
		t.Errorf("a token was minted with no vault: %+v", answer)
	}
	if len(answer.Warnings) != 1 {
		t.Errorf("the warnings did not travel: %+v", answer)
	}
}

func TestConnectRefusesAnAliasItWouldNotPutOnACommandLine(t *testing.T) {
	const secret = "the secret for this run"
	engine := connectEngine(t, ConnectHandlers{Secret: secret})
	for _, alias := range []string{"", "-oProxyCommand=id", "a b", "a;b"} {
		body := `{"alias":"` + alias + `"}`
		if code := send(t, engine, http.MethodPost, ConnectPath, body,
			map[string]string{handoff.HeaderName: secret}).Code; code != http.StatusBadRequest {
			t.Errorf("alias %q = %d, want 400", alias, code)
		}
	}
}

// メニューバーと終了確認が使う API であり、実行中の本数だけを数える。
// で、終了済みは残っていても数に入らない。閉じてよいかを問うための数だからだ。
func TestStatusAnswersWithTheLockAndTheLiveCount(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault, Sessions: func() int { return 3 },
	})

	recorder := send(t, engine, http.MethodGet, StatusPath, "",
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer CLIStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Vault || !answer.Unlocked || answer.Sessions != 3 {
		t.Fatalf("answer = %+v", answer)
	}
}

// handoff と status が異なる owner や protocol を名乗ると、CLI は別の engine に
// 接続したことを検出できない。合成の根が同じ値を route まで渡すことを検査する。
func TestCLIStatusIncludesOwner(t *testing.T) {
	const cliSecret = "the secret for this run"
	service := newCLIVaultService(t)
	server, err := New(Options{
		Listener:        fakeListener{address: &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 43123}},
		CLISecret:       cliSecret,
		Passwords:       service,
		Owner:           handoff.OwnerEngine,
		Version:         "v1.2.3-test",
		ProtocolVersion: handoff.ProtocolVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, StatusPath, nil)
	request.Host = "127.0.0.1:43123"
	request.Header.Set(handoff.HeaderName, cliSecret)
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var answer CLIStatus
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Owner != handoff.OwnerEngine || answer.Version != "v1.2.3-test" || answer.ProtocolVersion != handoff.ProtocolVersion {
		t.Fatalf("identity = owner %q, version %q, protocol %d", answer.Owner, answer.Version, answer.ProtocolVersion)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 6 {
		t.Fatalf("status fields = %v, want exact CLIStatus shape", raw)
	}
}

// vault が未作成ならパスワードを尋ねない。
func TestStatusSaysThereIsNoVaultToUnlock(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	// Initialise を呼ばない。新規インストール直後の姿である。
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret, Passwords: vault})

	recorder := send(t, engine, http.MethodGet, StatusPath, "",
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer CLIStatus
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Vault {
		t.Fatalf("answer = %+v, want no vault", answer)
	}
}

// handoff の秘密を持たないものには応答しない。
func TestStatusRefusesWithoutTheSecret(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: "the secret for this run"})
	if recorder := send(t, engine, http.MethodGet, StatusPath, "", nil); recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// liveSessions が実際に「実行中」を数えていることを、registry を組み立てずに見る。
// 終了済み（Exited が非 nil）を混ぜても、数に入るのは実行中ものだけである。
func TestLiveSessionsCountsOnlyTheOnesStillRunning(t *testing.T) {
	views := []terminal.View{
		{ID: "running-1"},
		{ID: "finished", Exited: &terminal.ExitInfo{Code: 0}},
		{ID: "running-2"},
		{ID: "running-3"},
	}
	if got := liveSessions(views); got != 3 {
		t.Fatalf("liveSessions = %d, want 3", got)
	}
}

// CLI からロック解除した状態は engine に保持され、Web UI でも共有される。
func TestUnlockOpensTheVaultFromTheCommandLine(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	vault.Lock()
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret, Passwords: vault})

	body := `{"passphrase":"` + testPassphrase + `"}`
	recorder := send(t, engine, http.MethodPost, VaultUnlockPath, body,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unlock = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !vault.Unlocked() {
		t.Fatal("the vault stayed locked")
	}
}

// 間違いは拒む。どう間違っていたかは言わない。
func TestUnlockRefusesTheWrongPassphrase(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	vault.Lock()
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret, Passwords: vault})

	recorder := send(t, engine, http.MethodPost, VaultUnlockPath, `{"passphrase":"wrong"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusUnauthorized || vault.Unlocked() {
		t.Fatalf("unlock = %d, unlocked = %v", recorder.Code, vault.Unlocked())
	}
}

// handoff の秘密を持たないものには応答しない。
func TestUnlockRefusesWithoutTheSecret(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: "the secret for this run"})
	if recorder := send(t, engine, http.MethodPost, VaultUnlockPath, `{"passphrase":"anything"}`, nil); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unlock = %d, want 401", recorder.Code)
	}
}

// 鍵のパスフレーズも、連鎖ぶん返る。
//
// ProxyJump の手前に立つホストは、それ自身が alias であり、そこにも別の鍵が
// 指定されうる。行き先のぶんだけを渡していた間、手前で止まる接続はそのたびに
// 手入力を求めていた。アカウントパスワードの側は最初から連鎖ぶんを渡している。
func TestKeyPassphrasesTravelForEveryHopOfTheConnection(t *testing.T) {
	const cliSecret = "the secret for this run"
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	for path, phrase := range map[string]string{"id_edge": "edge-phrase", "id_target": "target-phrase"} {
		if err := vault.SetCredential(secret.KindKeyPassphrase, path+"-name", phrase); err != nil {
			t.Fatal(err)
		}
		if err := vault.AssignCredential(secret.KindKeyPassphrase, path, path+"-name"); err != nil {
			t.Fatal(err)
		}
	}

	engine := connectEngine(t, ConnectHandlers{
		Secret: cliSecret, Passwords: vault,
		Aliases: func(alias string) []string { return []string{"edge", alias} },
		WorkspaceKeys: func(alias string) ([]string, error) {
			return map[string][]string{"edge": {"id_edge"}, "target": {"id_target"}}[alias], nil
		},
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"target"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusOK {
		t.Fatalf("connect = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Passphrases["id_edge"] != "edge-phrase" {
		t.Errorf("the jump host's key passphrase did not travel: %+v", answer)
	}
	if answer.Passphrases["id_target"] != "target-phrase" {
		t.Errorf("the destination's key passphrase did not travel: %+v", answer)
	}
}
