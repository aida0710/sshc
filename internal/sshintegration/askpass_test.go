// Package sshintegration は、本物の OpenSSH サーバーを必要とするテストを収める。
//
// ここにあるものはすべて、SSHC_TEST_SSH_ADDR がサーバーを指していない限りスキップ
// される。`make integration` はコンテナで起動して変数を設定し、CI も同じことをする。
//
// 要点は、どんな偽物にも確かめられない唯一のこと。すなわち、askpass ヘルパーが
// 渡したものを OpenSSH が受け取り、認証が通るということである。パスワード機能の
// それ以外 — プロンプトのルール、トークンの意味論、拒否の挙動 — は密閉された形で
// 網羅されているが、それらはすべて「こうあるべき」の記述にすぎない。ここで見るの
// は「実際にそうなるか」である。
//
// 差し替える本番コンポーネントは Terminal.app だけであり、しかもその AppleScript
// が実行したであろうコマンドそのもので差し替えている。ヘルパー、エンドポイント、
// vault、トークンは、出荷されるコードそのものだ。
package sshintegration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"crypto/rand"

	"sshc/internal/app"
	"sshc/internal/application"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/storage"
)

const (
	addressVariable   = "SSHC_TEST_SSH_ADDR"
	userVariable      = "SSHC_TEST_SSH_USER"
	passwordVariable  = "SSHC_TEST_SSH_PASSWORD"
	keyVariable       = "SSHC_TEST_SSH_KEY"
	keyPhraseVariable = "SSHC_TEST_SSH_KEY_PASSPHRASE"

	alias      = "integration"
	passphrase = "correct horse battery staple"
)

type target struct{ host, port, user, password string }

func requireTarget(t *testing.T) target {
	t.Helper()
	address := os.Getenv(addressVariable)
	if address == "" {
		t.Skipf("%s is not set; start a server with `make integration` to run this", addressVariable)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("%s = %q: %v", addressVariable, address, err)
	}
	return target{host: host, port: port, user: os.Getenv(userVariable), password: os.Getenv(passwordVariable)}
}

func helperPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "bin", "sshc"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the binary under test is missing; run `make build` first: %v", err)
	}
	return path
}

// buildHome は、唯一のホストがコンテナであり、そのホスト鍵がすでに既知である
// ワークスペースを書き出す。
//
// 検査を無効にするのではなく鍵をスキャンしているのは意図的である。この機能は
// ホスト鍵の問いに答えることを拒否するので、StrictHostKeyChecking を切るテストは、
// このアプリケーションが決して作らない設定を試していることに
// なってしまう。
func buildHome(t *testing.T, destination target) string {
	t.Helper()
	// OpenSSH renders key paths through %.100s. Keep this black-box fixture
	// below that bound so the production policy can bind the full path instead
	// of accepting an ambiguous truncated prefix.
	home, err := os.MkdirTemp("/tmp", "sshc-it-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "known_hosts"), hostKeyOf(t, destination), 0o600); err != nil {
		t.Fatal(err)
	}

	configuration := strings.Join([]string{
		"Host " + alias,
		"\tHostName " + destination.host,
		"\tPort " + destination.port,
		"\tUser " + destination.user,
		// 絶対パスにするのは、OpenSSH が ~ を HOME ではなく passwd データベースから
		// 展開するからである。相対 Include なら HOME の設定だけで足りるが、これには
		// 足りない。このスイートの初回 CI 実行が失敗したのがまさにそれだ。ssh は設定を
		// まったく読まず、alias をホスト名として解決しようと
		// した。
		"\tUserKnownHostsFile " + filepath.Join(root, "known_hosts"),
		// 要点はパスワードの経路なので、鍵による経路は塞いである。
		"\tPubkeyAuthentication no",
		"\tPreferredAuthentications password",
		"\tStrictHostKeyChecking yes",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

var (
	hostKeyMutex sync.Mutex
	hostKey      []byte
	hostKeyError error
	hostKeyKnown bool
)

// hostKeyOf は、パッケージ全体で一度だけサーバーのホスト鍵をスキャンする。
//
// 以前はテストごとにスキャンしていた。テストが四つあれば、テスト自身が張る接続に
// 加えて四回のスキャンになる。CI では最初の二つが通った直後に、三つ目と四つ目が
// 空で返ってきた。コンテナの sshd は、同時に開始する未認証接続の数を制限して
// おり、keyscan もそのひとつだからである。鍵は毎回同じ鍵なので、もう一度
// スキャンすることは、その予算を使う以外に何ひとつしていなかったことに
// なる。
//
// リトライを入れているのも、一般的な不安定さ対策ではなく同じ理由からである。
// ここでの拒否は「いまはだめ」という意味であり、1 秒待てばそれではなくなる。
// すべての試行が拒否された場合は、それを覆い隠さずに報告する。
func hostKeyOf(t *testing.T, destination target) []byte {
	t.Helper()
	hostKeyMutex.Lock()
	defer hostKeyMutex.Unlock()
	if hostKeyKnown {
		if hostKeyError != nil {
			t.Fatalf("ssh-keyscan produced nothing: %v", hostKeyError)
		}
		return hostKey
	}

	empty := t.TempDir()
	for attempt := range 5 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		scan := exec.Command("ssh-keyscan", "-p", destination.port, destination.host)
		scan.Env = []string{"HOME=" + empty, "PATH=" + os.Getenv("PATH")}
		scanned, err := scan.Output()
		switch {
		case err != nil:
			hostKeyError = err
		case len(scanned) == 0:
			hostKeyError = errors.New("the scan succeeded but returned no key")
		default:
			hostKey, hostKeyError = scanned, nil
		}
		if hostKeyError == nil {
			break
		}
	}
	hostKeyKnown = true
	if hostKeyError != nil {
		t.Fatalf("ssh-keyscan produced nothing after 5 attempts: %v", hostKeyError)
	}
	return hostKey
}

// countingListener は、サーバーが受け付けた接続を数える。
//
// これは、「この vault が持つパスワードを ssh が拒否した」のか「ヘルパーがそもそも
// 尋ねなかった」のかをテストが見分ける唯一の手段である。どちらも Permission denied
// で終わる。ある CI 実行では、ヘルパーが答える代わりにアプリケーションの二つ目の
// コピーを起動していたにもかかわらず、ここの否定テストはすべて通ってしまった。
// SSH_ASKPASS はプログラムを指定し、OpenSSH はプロンプトだけを引数にそれを exec
// する。バイナリの方は、決して届くはずのないサブコマンドの語を探していた。
type countingListener struct {
	net.Listener
	connections atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	connection, err := l.Listener.Accept()
	if err == nil {
		l.connections.Add(1)
	}
	return connection, err
}

func requireTheHelperAsked(t *testing.T, listener *countingListener, before int64, output string) {
	t.Helper()
	if listener.connections.Load() <= before {
		t.Fatalf("the helper never reached this application, so nothing about the password was tested:\n%s", output)
	}
}

// startServer は、本物の /askpass エンドポイントを持つ本物の HTTP サーバーを動かす。
func startServer(t *testing.T, home string) (*secret.Service, *application.Service, *keys.Service, string, string, *countingListener) {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	vault := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := vault.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	configService := application.NewService(
		workspace,
		storage.NewManager(workspace, time.Now, rand.Reader),
	)
	keyService := keys.NewService(keys.ServiceOptions{
		Workspace: workspace, Transactions: storage.NewManager(workspace, time.Now, rand.Reader),
		Resolver: storage.NewResolver(workspace), Catalogue: keys.CatalogueReader{},
		Agent: platform.KeyAgent(nil), Now: time.Now, Random: rand.Reader,
	})

	bare, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &countingListener{Listener: bare}
	sessions, _, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server, err := httpserver.New(httpserver.Options{
		Listener:   listener,
		Sessions:   sessions,
		UI:         os.DirFS(home),
		Version:    "integration",
		Passwords:  vault,
		CLISecret:  "integration-cli-secret",
		Config:     configService,
		Keys:       keyService,
		Answerable: func(string, string) bool { return false },
		KeyPassphraseAnswerable: app.KeyPassphraseAnswerable(func(alias string) (application.DirectKeyPassphraseTarget, bool, error) {
			return configService.DirectKeyPassphraseTarget(alias, keyService.Inventory)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	serveContext, stop := context.WithCancel(context.Background())
	go func() { _ = server.Serve(serveContext) }()
	t.Cleanup(stop)
	// server.URL() はワンタイムのブートストラップフラグメントを含む。askpass の
	// エンドポイントに必要なのはオリジンだけで、セッショントークンを渡してはならない。
	origin := server.URL()
	if index := strings.Index(origin, "#"); index >= 0 {
		origin = origin[:index]
	}
	return vault, configService, keyService, origin,
		strings.TrimSuffix(origin, "/") + httpserver.AskpassPath, listener
}

func issueThroughCLI(t *testing.T, origin string) (token, kind, endpoint, identityFile, sshConfig string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, origin+httpserver.ConnectPath,
		strings.NewReader(`{"alias":"`+alias+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, "integration-cli-secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("cli connect = %s", response.Status)
	}
	var answer struct {
		AskpassToken string `json:"askpassToken"`
		AskpassKind  string `json:"askpassKind"`
		AskpassURL   string `json:"askpassUrl"`
		IdentityFile string `json:"identityFile"`
		SSHConfig    string `json:"sshConfig"`
	}
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	return answer.AskpassToken, answer.AskpassKind, answer.AskpassURL, answer.IdentityFile, answer.SSHConfig
}

func runSSH(t *testing.T, home, endpoint, token string, arguments ...string) (string, error) {
	return runSSHWithKind(t, home, endpoint, token, "password", arguments...)
}

func runSSHWithKind(t *testing.T, home, endpoint, token, kind string, arguments ...string) (string, error) {
	return runSSHWithCredential(t, home, endpoint, token, kind, "", "", arguments...)
}

func runSSHWithCredential(t *testing.T, home, endpoint, token, kind, identityFile, sshConfig string, arguments ...string) (string, error) {
	t.Helper()
	// TerminalPasswordScript がシェルに実行させるものそのもの。
	// -F を明示するのは UserKnownHostsFile と同じ理由である。既定のユーザー設定の
	// パスは HOME ではなく passwd データベースから来るので、HOME しか設定しない
	// テストは、設定なしで黙って走ってしまう。
	configPath := filepath.Join(home, ".ssh", "config")
	if sshConfig != "" {
		file, err := os.CreateTemp("", "sshc-integration-connect-*.conf")
		if err != nil {
			t.Fatal(err)
		}
		configPath = file.Name()
		if err := file.Chmod(0o600); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if _, err := file.WriteString(sshConfig); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(configPath) })
	}
	sshArguments := []string{
		"-F", configPath,
		"-o", "NumberOfPasswordPrompts=1",
		"-o", "BatchMode=no",
	}
	if identityFile != "" {
		sshArguments = append(sshArguments, "-i", identityFile, "-o", "IdentitiesOnly=yes")
	}
	sshArguments = append(sshArguments, "--", alias)
	command := exec.Command("ssh", append(sshArguments, arguments...)...)
	command.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"SSH_ASKPASS=" + helperPath(t),
		"SSH_ASKPASS_REQUIRE=force",
		"SSHC_ASKPASS_URL=" + endpoint,
		"SSHC_ASKPASS_TOKEN=" + token,
		"SSHC_ASKPASS_ALIAS=" + alias,
		"SSHC_ASKPASS_KIND=" + kind,
	}
	if identityFile != "" {
		command.Env = append(command.Env, "SSHC_ASKPASS_KEY_PATH="+identityFile)
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestTheHelperDecryptsARealPrivateKeyWithItsStoredPassphrase(t *testing.T) {
	destination := requireTarget(t)
	sourceKey := os.Getenv(keyVariable)
	keyPassphrase := os.Getenv(keyPhraseVariable)
	if sourceKey == "" || keyPassphrase == "" {
		t.Skipf("%s and %s are not set; the integration target has no encrypted key fixture", keyVariable, keyPhraseVariable)
	}
	home := buildHome(t, destination)
	keyPath := filepath.Join(home, ".ssh", "id_integration")
	keyBytes, err := os.ReadFile(sourceKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, ".ssh", "config")
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	configuration = []byte(strings.ReplaceAll(string(configuration),
		"\tPubkeyAuthentication no\n\tPreferredAuthentications password\n",
		"\tPubkeyAuthentication yes\n\tPreferredAuthentications publickey\n\tIdentityFile "+keyPath+"\n"))
	if err := os.WriteFile(configPath, configuration, 0o600); err != nil {
		t.Fatal(err)
	}
	vault, configService, keyService, origin, endpoint, listener := startServer(t, home)
	if err := vault.SetCredential(secret.KindKeyPassphrase, "integration-key", keyPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := vault.AssignCredential(secret.KindKeyPassphrase, "id_integration", "integration-key"); err != nil {
		t.Fatal(err)
	}
	target, ok, err := configService.DirectKeyPassphraseTarget(alias, keyService.Inventory)
	if err != nil || !ok {
		t.Fatalf("production key target = %+v, ok %v, err %v", target, ok, err)
	}
	token, kind, issuedEndpoint, identityFile, sshConfig := issueThroughCLI(t, origin)
	if token == "" || kind != "key_passphrase" || issuedEndpoint != endpoint {
		t.Fatalf("CLI issue = token %t, kind %q, endpoint %q", token != "", kind, issuedEndpoint)
	}
	if identityFile != target.PromptPath || sshConfig != target.ConfigSnapshot {
		t.Fatalf("CLI target = identity %q, config bytes %d; want %q / %d", identityFile, len(sshConfig), target.PromptPath, len(target.ConfigSnapshot))
	}

	asked := listener.connections.Load()
	output, err := runSSHWithCredential(t, home, endpoint, token, "key_passphrase", identityFile, sshConfig,
		"echo", "authenticated-by-key-passphrase")
	if err != nil {
		t.Fatalf("ssh = %v\n%s", err, output)
	}
	requireTheHelperAsked(t, listener, asked, output)
	if !strings.Contains(output, "authenticated-by-key-passphrase") {
		t.Fatalf("the remote command did not run:\n%s", output)
	}
}

func TestAStoredAccountPasswordIsNeverReturnedToOpenSSH(t *testing.T) {
	destination := requireTarget(t)
	home := buildHome(t, destination)
	vault, _, _, _, endpoint, listener := startServer(t, home)
	if err := vault.Set(alias, destination.password); err != nil {
		t.Fatal(err)
	}
	token, err := vault.IssueToken(alias)
	if err != nil {
		t.Fatal(err)
	}

	asked := listener.connections.Load()
	output, err := runSSH(t, home, endpoint, token, "true")
	if err == nil {
		t.Fatalf("a stored account password authenticated despite the disabled policy:\n%s", output)
	}
	// The production helper only presents key-passphrase tokens to the server.
	// A legacy account-password token must be rejected locally, before the
	// bearer token or its stored value reaches the HTTP endpoint.
	if got := listener.connections.Load(); got != asked {
		t.Fatalf("the helper presented an account-password token to the application: connections %d -> %d\n%s",
			asked, got, output)
	}
	requireAuthenticationWasAttempted(t, output)
}

// requireAuthenticationWasAttempted は、テストが誤った理由で通るのを防ぐ。
//
// ここの否定ケースはいずれも ssh が失敗したことを表明するが、ssh はさまざまな理由
// で失敗する。このスイートの初回 CI 実行では、ssh がサーバーにまったく到達して
// いないのに二つが通っていた — 設定を一度も読んでいなかったため、alias を解決
// できなかったのである。「パスワードが拒否された」と「そもそも接続がなかった」を
// 区別できないテストは、パスワードを試験していない。
func requireAuthenticationWasAttempted(t *testing.T, output string) {
	t.Helper()
	for _, symptom := range []string{
		"Could not resolve hostname",
		"Connection refused",
		"No route to host",
		"Host key verification failed",
	} {
		if strings.Contains(output, symptom) {
			t.Fatalf("ssh failed before authentication (%s):\n%s", symptom, output)
		}
	}
	if !strings.Contains(output, "Permission denied") {
		t.Fatalf("ssh did not report a refused authentication, so it may have failed earlier:\n%s", output)
	}
}
