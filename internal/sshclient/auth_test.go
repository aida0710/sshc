package sshclient_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/keys"
	"sshc/internal/sshclient"
)

// scriptedPrompter は、決められた結果を順に返す。何を尋ねられたかを記録する。
type scriptedPrompter struct {
	answers  []string
	confirm  bool
	asked    []string
	secretly []string
}

func (p *scriptedPrompter) next(prompt string) (string, error) {
	p.asked = append(p.asked, prompt)
	if len(p.answers) == 0 {
		return "", sshclient.ErrPromptAborted
	}
	answer := p.answers[0]
	p.answers = p.answers[1:]
	return answer, nil
}

func (p *scriptedPrompter) Line(prompt string) (string, error) { return p.next(prompt) }

func (p *scriptedPrompter) Secret(prompt string) (string, error) {
	p.secretly = append(p.secretly, prompt)
	return p.next(prompt)
}

func (p *scriptedPrompter) Confirm(prompt string) (bool, error) {
	p.asked = append(p.asked, prompt)
	return p.confirm, nil
}

// writeKey は、鍵ひとつをフィクスチャのディレクトリへ書き、そのパスを返す。
func writeKey(t *testing.T, directory, name string, passphrase []byte) (string, ssh.Signer) {
	t.Helper()
	private, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := keys.EncodePrivateKey(private, "fixture", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return path, signer
}

// connect は、この認証だけを使ってテストサーバーへ繋ぐ。
//
// 本物のハンドシェイクである。通ったかどうかを決めるのはサーバーであり、
// このテストが「通したことにする」余地はない。
func connect(t *testing.T, server *testServer, target sshclient.Target, auth sshclient.Auth, prompt sshclient.Prompter) error {
	t.Helper()
	conn := server.Dial()
	client, channels, requests, err := ssh.NewClientConn(conn, target.Address(), &ssh.ClientConfig{
		User:            target.User,
		Auth:            auth.Methods(target, prompt),
		HostKeyCallback: ssh.FixedHostKey(server.HostKey.PublicKey()),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		return err
	}
	go ssh.DiscardRequests(requests)
	go func() {
		for channel := range channels {
			_ = channel.Reject(ssh.UnknownChannelType, "not needed")
		}
	}()
	return client.Close()
}

func targetWith(server *testServer, identities ...string) sshclient.Target {
	return sshclient.Target{
		Alias: "bastion", HostName: server.Host(), Port: server.Port(), User: "ops",
		Identities: identities, Methods: sshclient.DefaultMethods(),
	}
}

func TestAKeyWithoutAPassphraseAuthenticates(t *testing.T) {
	home := t.TempDir()
	path, signer := writeKey(t, home, "id_ed25519", nil)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{signer.PublicKey()}})
	prompt := &scriptedPrompter{}

	if err := connect(t, server, targetWith(server, path), sshclient.Auth{}, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Errorf("a key without a passphrase still asked: %#v", prompt.asked)
	}
}

// 保存されているパスフレーズを先に試す。結果を既に持っているなら尋ねない。
func TestAStoredPassphraseIsUsedWithoutAsking(t *testing.T) {
	home := t.TempDir()
	path, signer := writeKey(t, home, "id_ed25519", []byte("correct horse"))
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{signer.PublicKey()}})
	prompt := &scriptedPrompter{}
	auth := sshclient.Auth{Stored: func(asked string) (string, bool) {
		if asked != path {
			return "", false
		}
		return "correct horse", true
	}}

	if err := connect(t, server, targetWith(server, path), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Errorf("a stored passphrase still asked the user: %#v", prompt.asked)
	}
}

// 保存されていなければ端末で尋ねる。尋ねるのは Secret でなければならない。
func TestAnUnstoredPassphraseIsAskedWithoutEchoing(t *testing.T) {
	home := t.TempDir()
	path, signer := writeKey(t, home, "id_ed25519", []byte("correct horse"))
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{signer.PublicKey()}})
	prompt := &scriptedPrompter{answers: []string{"correct horse"}}

	if err := connect(t, server, targetWith(server, path), sshclient.Auth{}, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 {
		t.Fatalf("the passphrase was not asked in secret: %#v", prompt.asked)
	}
	if !strings.Contains(prompt.secretly[0], path) {
		t.Errorf("the prompt does not name the key: %q", prompt.secretly[0])
	}
}

// 間違えても諦めない。上限は OpenSSH と同じ 3 回。
func TestAWrongPassphraseIsAskedAgainAndThenGivesUp(t *testing.T) {
	home := t.TempDir()
	path, signer := writeKey(t, home, "id_ed25519", []byte("correct horse"))
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{signer.PublicKey()}})

	retried := &scriptedPrompter{answers: []string{"wrong", "correct horse"}}
	if err := connect(t, server, targetWith(server, path), sshclient.Auth{}, retried); err != nil {
		t.Fatalf("a corrected passphrase did not connect: %v", err)
	}
	if len(retried.secretly) != 2 {
		t.Errorf("asked %d times, want 2", len(retried.secretly))
	}

	exhausted := &scriptedPrompter{answers: []string{"no", "still no", "nope", "correct horse"}}
	if err := connect(t, server, targetWith(server, path), sshclient.Auth{}, exhausted); err == nil {
		t.Fatal("three wrong passphrases still connected")
	}
	if len(exhausted.secretly) != 3 {
		t.Errorf("asked %d times, want the ceiling of 3", len(exhausted.secretly))
	}
}

func TestPasswordAuthenticationAsksTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	prompt := &scriptedPrompter{answers: []string{"hunter2"}}
	target := targetWith(server)

	if err := connect(t, server, target, sshclient.Auth{}, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 {
		t.Fatalf("the password was not asked in secret: %#v", prompt.asked)
	}
}

func TestKeyboardInteractiveCarriesTheServerQuestions(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "123456"}})
	prompt := &scriptedPrompter{answers: []string{"123456"}}

	if err := connect(t, server, targetWith(server), sshclient.Auth{}, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 || !strings.Contains(prompt.secretly[0], "Verification code") {
		t.Fatalf("the server's own question did not reach the user: %#v", prompt.secretly)
	}
}

// IdentitiesOnly yes は、設定に書かれた鍵だけを使うという指定である。
func TestIdentitiesOnlySkipsTheAgent(t *testing.T) {
	home := t.TempDir()
	socket, agentKey := runTestAgent(t, home)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{agentKey.PublicKey()}})

	target := targetWith(server)
	target.IdentitiesOnly = true
	// 鍵がひとつも無く agent も使えないので、公開鍵認証は提示されない。
	auth := sshclient.Auth{AgentSocket: socket}
	if err := connect(t, server, target, auth, &scriptedPrompter{}); err == nil {
		t.Fatal("IdentitiesOnly still used the agent's key")
	}

	// 同じ設定で IdentitiesOnly を外せば通る。上の失敗が「agent が壊れていた」
	// ではなく「使わなかった」ことの証拠である。
	target.IdentitiesOnly = false
	if err := connect(t, server, target, auth, &scriptedPrompter{}); err != nil {
		t.Fatalf("the agent's key did not connect: %v", err)
	}
}

// 鍵も agent も無い接続は、公開鍵認証を提示しない。OpenSSH の既定の探索順
// （~/.ssh/id_ed25519 など）は持たない。
func TestWithoutAnyKeyPublicKeyAuthenticationIsNotOffered(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	target := targetWith(server)
	target.Methods = sshclient.Methods{PublicKey: true}

	methods := sshclient.Auth{}.Methods(target, &scriptedPrompter{})
	if len(methods) != 0 {
		t.Fatalf("methods = %d, want none", len(methods))
	}
}

// 尋ねる手段が無ければ、尋ねる方式は提示しない。UI の無い経路でユーザーを待つと、
// その接続は永久に終わらない。
func TestWithoutAPrompterOnlyPublicKeyIsOffered(t *testing.T) {
	home := t.TempDir()
	path, _ := writeKey(t, home, "id_ed25519", nil)
	target := sshclient.Target{Identities: []string{path}, Methods: sshclient.DefaultMethods()}

	methods := sshclient.Auth{}.Methods(target, nil)
	if len(methods) != 1 {
		t.Fatalf("methods = %d, want only the public key method", len(methods))
	}
}

func TestAMissingKeyFileIsReportedRatherThanIgnored(t *testing.T) {
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{newHostKey(t).PublicKey()}})
	target := targetWith(server, filepath.Join(t.TempDir(), "absent"))
	target.Methods = sshclient.Methods{PublicKey: true}

	err := connect(t, server, target, sshclient.Auth{}, &scriptedPrompter{})
	if err == nil {
		t.Fatal("a missing key file still connected")
	}
	if !strings.Contains(err.Error(), "absent") {
		t.Errorf("the failure does not name the key it could not read: %v", err)
	}
}

func TestNoIdentityIsItsOwnError(t *testing.T) {
	home := t.TempDir()
	target := sshclient.Target{Identities: []string{filepath.Join(home, "absent")}}
	_, err := sshclient.Auth{}.Signers(target, nil)
	if !errors.Is(err, sshclient.ErrNoIdentity) {
		t.Fatalf("Signers = %v, want ErrNoIdentity", err)
	}
}

// runTestAgent は、プロセス内の ssh-agent を unix ソケットで待ち受けさせる。
func runTestAgent(t *testing.T, _ string) (string, ssh.Signer) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: private}); err != nil {
		t.Fatal(err)
	}

	// t.TempDir() は使わない。unix ソケットのパスには 100 バイト程度の
	// 上限があり、テスト名を含むあの長いパスは macOS でそれを超える。超えると
	// bind が失敗し、この検査は skip として静かに消える。
	socketDirectory, err := os.MkdirTemp("", "sshc-agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDirectory) })

	socket := filepath.Join(socketDirectory, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen on %q: %v", socket, err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { _ = agent.ServeAgent(keyring, conn) }()
		}
	}()
	return socket, signer
}

// 保管庫に置いてあるのに毎回尋ねるなら、置く意味が無い。
func TestAStoredPasswordAnswersWithoutAskingTheUser(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	// 結果を持たない。尋ねられた時点でこの接続は失敗する。
	prompt := &scriptedPrompter{}
	auth := sshclient.Auth{Password: func(target sshclient.Target) (string, bool) {
		return "hunter2", target.Alias == "bastion"
	}}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Fatalf("the stored password was not used: %#v", prompt.asked)
	}
}

// 普通の Linux はパスワードを keyboard-interactive で聞いてくる。問いがひとつで
// 画面に出さないなら、それはパスワードを聞かれている形である。
func TestAStoredPasswordAnswersASingleHiddenQuestion(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Password: ": "hunter2"}})
	prompt := &scriptedPrompter{}
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Fatalf("the stored password was not used: %#v", prompt.asked)
	}
}

// 問いが複数あるものに、保存されたパスワードを差し出す意味は無い。2FA の
// 二つ目の問いに対して、それは間違った結果である。
func TestAStoredPasswordDoesNotAnswerATwoQuestionChallenge(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{
		"Password: ": "hunter2", "Verification code: ": "123456",
	}})
	prompt := &scriptedPrompter{answers: []string{"one", "two"}}
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }}

	_ = connect(t, server, targetWith(server), auth, prompt)

	if len(prompt.asked) < 2 {
		t.Fatalf("the server's two questions did not reach the user: %#v", prompt.asked)
	}
}

// 保存された結果は古いことがある。断られたらユーザーに尋ね直す。一度で諦めると、
// その alias は保管庫を直すまで開けなくなる。
func TestAStaleStoredPasswordStillLetsTheUserAnswer(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	prompt := &scriptedPrompter{answers: []string{"hunter2"}}
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "what it used to be", true }}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 {
		t.Fatalf("the user was never asked after the stored password was refused: %#v", prompt.secretly)
	}
}
