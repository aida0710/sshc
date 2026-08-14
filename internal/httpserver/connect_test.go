package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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
// **名前空間が別だからである。** 混ぜれば、ローカルの鍵を開くための秘密が
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
	if answer.Passphrase != "" {
		t.Fatalf("an account password came back as a key passphrase: %+v", answer)
	}
	// 自分の欄には載る。**保存してあるのに使わないなら、保存させる意味が無い。**
	if answer.Passwords["bastion"] != "legacy-password" {
		t.Fatalf("the stored account password did not reach the command line: %+v", answer)
	}
}

// 保存済みアカウントパスワードだけを持つ alias は、鍵についての答えを持たない。
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
	if answer.Passphrase != "" || answer.KeyPath != "" {
		t.Fatalf("a stored account password produced a key answer: %+v", answer)
	}
	if answer.Passwords["password-only"] != "stored-account-password" {
		t.Fatalf("the stored account password did not reach the command line: %+v", answer)
	}
}

// 連鎖ぶんを返す。**ProxyJump の手前に立つホストも、それ自身が alias である。**
//
// 同時に、返すのはこの接続に現れるものだけである——保管庫の一覧にはしない。
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
	// **尋ねられた接続に現れないものは返さない。**
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
		KeyPassphraseTarget: func(alias string) (string, string, string, string, bool, error) {
			return "id_ed25519_server", filepath.Join(home, ".ssh", "id_ed25519_server"),
				"Host bastion\n\tIdentityFile " + filepath.Join(home, ".ssh", "id_ed25519_server") + "\n",
				"evidence", alias == "bastion", nil
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
	// **答えそのものが返る。** 単回トークンにしていたのは、引き換える相手が
	// 別のプログラムだったからである。要求を出した本人が受け取るなら、
	// 発行と引き換えを分ける理由が無い。
	if answer.KeyPath != "id_ed25519_server" {
		t.Fatalf("connect answer = %+v, want the workspace-relative key path", answer)
	}
	if answer.Passphrase == "" {
		t.Fatalf("connect answer = %+v, want the stored passphrase", answer)
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
		KeyPassphraseTarget: func(string) (string, string, string, string, bool, error) {
			return "", "", "", "", false, os.ErrPermission
		},
	})
	recorder := send(t, engine, http.MethodPost, ConnectPath, `{"alias":"bastion"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	var answer connectResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Passphrase != "" {
		t.Fatal("an unreadable key policy fell back to the account-password namespace")
	}
}

// secret がなければ、このエンドポイントは何も語らない——alias が
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

// vault がない場合の答えはトークンなしの接続であり、それは正常な接続である。
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
	if answer.Passphrase != "" {
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

// 止める要求は、答えてから止める。**止めてから答えると、呼んだ側は成功と
// 切断を区別できない。**
func TestStopAnswersAndThenClosesTheStopChannel(t *testing.T) {
	const cliSecret = "the secret for this run"
	stopped := make(chan struct{})
	var once sync.Once

	engine := connectEngine(t, ConnectHandlers{
		Secret:   cliSecret,
		Shutdown: func() { once.Do(func() { close(stopped) }) },
	})

	refused := send(t, engine, http.MethodPost, StopPath, "{}", nil)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("stop without the secret = %d, want 403", refused.Code)
	}
	select {
	case <-stopped:
		t.Fatal("a refused request still stopped the engine")
	default:
	}

	accepted := send(t, engine, http.MethodPost, StopPath, "{}",
		map[string]string{handoff.HeaderName: cliSecret})
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("stop = %d, want 202", accepted.Code)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the engine was never asked to stop")
	}
}

// 止める手段が配線されていなければ、止まったふりをしない。
func TestStopSaysSoWhenThereIsNoWayToStop(t *testing.T) {
	const cliSecret = "the secret for this run"
	engine := connectEngine(t, ConnectHandlers{Secret: cliSecret})

	response := send(t, engine, http.MethodPost, StopPath, "{}",
		map[string]string{handoff.HeaderName: cliSecret})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("stop without a shutdown = %d, want 503", response.Code)
	}
}

// メニューバーと終了時の確認が読む口である。**数えるのは生きている本数だけ**
// で、終了済みは残っていても数に入らない——閉じてよいかを問うための数だからだ。
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
	var answer statusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Unlocked || answer.Sessions != 3 {
		t.Fatalf("answer = %+v", answer)
	}
}

// handoff の秘密を持たないものには答えない。
func TestStatusRefusesWithoutTheSecret(t *testing.T) {
	engine := connectEngine(t, ConnectHandlers{Secret: "the secret for this run"})
	if recorder := send(t, engine, http.MethodGet, StatusPath, "", nil); recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

// liveSessions が実際に「生きている」を数えていることを、registry を組み立てずに見る。
// 終了済み（Exited が非 nil）を混ぜても、数に入るのは生きているものだけである。
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

// **端末でも解錠できる。** ブラウザを開かずに答えられることが、この口の理由で
// ある。解錠はエンジンの中に残るので、あとで窓を開けば解錠済みである。
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
	recorder := send(t, engine, http.MethodPost, UnlockPath, body,
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

	recorder := send(t, engine, http.MethodPost, UnlockPath, `{"passphrase":"wrong"}`,
		map[string]string{handoff.HeaderName: cliSecret})
	if recorder.Code != http.StatusForbidden || vault.Unlocked() {
		t.Fatalf("unlock = %d, unlocked = %v", recorder.Code, vault.Unlocked())
	}
}
