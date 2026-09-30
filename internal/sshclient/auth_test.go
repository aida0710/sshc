package sshclient_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	if !strings.Contains(prompt.secretly[0], "ops@"+server.Address()) ||
		!strings.Contains(prompt.secretly[0], "bastion") {
		t.Fatalf("the password prompt did not identify its SSH hop: %q", prompt.secretly[0])
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

func TestStoredTOTPAnswersAnExplicitChallengeWithoutPrompting(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "123456"}})
	prompt := &scriptedPrompter{}
	auth := sshclient.Auth{TOTP: func(target sshclient.Target, question string) (string, bool) {
		if target.Alias != "bastion" || question != "Verification code: " {
			return "", false
		}
		return "123456", true
	}}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Fatalf("stored TOTP still prompted: %#v", prompt.asked)
	}
}

// 拒否は設定か資格情報を直すまで続くので、呼び出し側が型で見分けて再試行を止める。
func TestARefusedKeyIsReportedAsAnAuthenticationRejection(t *testing.T) {
	_, _, accepted := keyPair(t)
	path, contents, _ := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{accepted}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	_, err := dialerFor(t, server, auth).Run(context.Background(), targetWith(server, path), sshclient.Command{Line: "true"})
	if !errors.Is(err, sshclient.ErrAuthenticationRejected) {
		t.Fatalf("Run = %v, want an authentication rejection", err)
	}
	if !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("the rejection lost the server's reason: %q", err.Error())
	}
}

// エージェントに鍵を多く入れていると、サーバーは「no supported methods remain」に
// なる前に MaxAuthTries で接続を切る。これも設定を直すまで続く拒否である。
func TestAServerThatDisconnectsAfterTooManyRefusedKeysIsReportedAsAnAuthenticationRejection(t *testing.T) {
	// サーバーの MaxAuthTries より多くの鍵を差し出す。
	const maxAuthTries, refusedKeys = 2, 4
	_, _, accepted := keyPair(t)
	offered := map[string][]byte{}
	paths := make([]string, 0, refusedKeys)
	for index := range refusedKeys {
		_, contents, _ := keyPair(t)
		path := "/keys/refused_" + strconv.Itoa(index)
		offered[path] = contents
		paths = append(paths, path)
	}
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{accepted}, MaxAuthTries: maxAuthTries})
	auth := sshclient.Auth{ReadFile: func(path string) ([]byte, error) { return offered[path], nil }}

	_, err := dialerFor(t, server, auth).Run(context.Background(), targetWith(server, paths...), sshclient.Command{Line: "true"})
	if !errors.Is(err, sshclient.ErrAuthenticationRejected) {
		t.Fatalf("Run = %v, want an authentication rejection", err)
	}
	if !strings.Contains(err.Error(), "too many authentication failures") {
		t.Fatalf("the rejection lost the server's reason: %q", err.Error())
	}
}

// 断られたコードを送り直しても、同じ時間窓では同じコードになり、また断られる。
func TestARejectedStoredTOTPAsksTheUserInsteadOfSendingItAgain(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "654321"}})
	prompt := &scriptedPrompter{answers: []string{"654321"}}
	generated := 0
	auth := sshclient.Auth{TOTP: func(sshclient.Target, string) (string, bool) {
		generated++
		return "123456", true
	}}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if generated != 1 {
		t.Fatalf("the stored TOTP was generated %d times in one connection", generated)
	}
	if len(prompt.secretly) != 1 || !strings.Contains(prompt.secretly[0], "Saved verification code was rejected") {
		t.Fatalf("the user was not asked after the stored code was refused: %#v", prompt.secretly)
	}
}

func TestANonInteractiveConnectionStopsAfterItsStoredTOTPIsRejected(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "654321"}})
	generated := 0
	auth := sshclient.Auth{TOTP: func(sshclient.Target, string) (string, bool) {
		generated++
		return "123456", true
	}}

	_, err := dialerFor(t, server, auth).Run(context.Background(), targetWith(server), sshclient.Command{Line: "true"})
	if !errors.Is(err, sshclient.ErrPromptUnavailable) {
		t.Fatalf("Run = %v, want the refusal to ask", err)
	}
	if generated != 1 {
		t.Fatalf("the stored TOTP was generated %d times in one connection", generated)
	}
}

// storedTOTPAndPassword は、拒否されるTOTPと、取り出された回数を数える保存済み
// パスワードの両方を持つ Auth を作る。
func storedTOTPAndPassword(passwordReleased *int) sshclient.Auth {
	return sshclient.Auth{
		TOTP: func(sshclient.Target, string) (string, bool) { return "123456", true },
		Password: func(sshclient.Target) (string, bool) {
			*passwordReleased++
			return "saved-account-password", true
		},
	}
}

func answersContain(answers [][]string, wanted string) bool {
	for _, round := range answers {
		for _, answer := range round {
			if answer == wanted {
				return true
			}
		}
	}
	return false
}

// TOTPの質問にアカウントのパスワードを答えると、パスワードが別の用途の欄へ
// 送られ、正しい保存値が拒否されたと表示される。
func TestARejectedStoredTOTPQuestionIsNeverAnsweredWithTheSavedPassword(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "654321"}})
	prompt := &scriptedPrompter{answers: []string{"654321"}}
	passwordReleased := 0

	if err := connect(t, server, targetWith(server), storedTOTPAndPassword(&passwordReleased), prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if passwordReleased != 0 {
		t.Fatalf("the saved password was released %d times to a TOTP question", passwordReleased)
	}
	if answersContain(server.KeyboardAnswers(), "saved-account-password") {
		t.Fatalf("the saved password reached the server as a verification code: %#v", server.KeyboardAnswers())
	}
	if len(prompt.secretly) != 1 || strings.Contains(prompt.secretly[0], "Saved password was rejected") {
		t.Fatalf("the user was told about a password that was never sent: %#v", prompt.secretly)
	}
}

func TestAVerificationCodeQuestionWithoutAStoredTOTPAsksTheUserInsteadOfSendingTheSavedPassword(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "654321"}})
	prompt := &scriptedPrompter{answers: []string{"654321"}}
	passwordReleased := 0
	auth := storedTOTPAndPassword(&passwordReleased)
	auth.TOTP = func(sshclient.Target, string) (string, bool) { return "", false }

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if passwordReleased != 0 {
		t.Fatalf("the saved password was released %d times to a TOTP question", passwordReleased)
	}
	if len(prompt.secretly) != 1 {
		t.Fatalf("the user was not asked for the verification code: %#v", prompt.secretly)
	}
}

func TestANonInteractiveConnectionSendsOneAnswerWhenItsStoredTOTPIsRejected(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Verification code: ": "654321"}})
	passwordReleased := 0

	_, err := dialerFor(t, server, storedTOTPAndPassword(&passwordReleased)).
		Run(context.Background(), targetWith(server), sshclient.Command{Line: "true"})
	if !errors.Is(err, sshclient.ErrPromptUnavailable) {
		t.Fatalf("Run = %v, want the refusal to ask", err)
	}
	if passwordReleased != 0 {
		t.Fatalf("the saved password was released %d times to a TOTP question", passwordReleased)
	}
	if answers := server.KeyboardAnswers(); len(answers) != 1 {
		t.Fatalf("the server received %d rounds of answers, want only the stored TOTP: %#v", len(answers), answers)
	}
}

func TestStoredTOTPAnswersAnExplicitEchoedChallengeWithoutPrompting(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Keyboard:     map[string]string{"Verification code: ": "123456"},
		KeyboardEcho: true, KeyboardName: "tsukuba",
	})
	prompt := &scriptedPrompter{}
	var observed sshclient.CredentialEvent
	var echoed bool
	auth := sshclient.Auth{
		TOTP: func(target sshclient.Target, question string) (string, bool) {
			if target.Alias != "bastion" || question != "Verification code: " {
				return "", false
			}
			return "123456", true
		},
		ObserveCredential: func(_ sshclient.Target, event sshclient.CredentialEvent, shown bool) {
			observed, echoed = event, shown
		},
	}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Fatalf("echoed stored TOTP still prompted: %#v", prompt.asked)
	}
	if observed != sshclient.CredentialTOTPUsed || !echoed {
		t.Fatalf("credential observation = %q, echoed %t", observed, echoed)
	}
}

func TestUnavailableStoredTOTPIsReportedWithoutSuppressingManualEntry(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Keyboard:     map[string]string{"Verification code: ": "123456"},
		KeyboardEcho: true, KeyboardName: "tsukuba",
	})
	prompt := &scriptedPrompter{answers: []string{"123456"}}
	var observed sshclient.CredentialEvent
	auth := sshclient.Auth{
		TOTP: func(sshclient.Target, string) (string, bool) { return "", false },
		ObserveCredential: func(_ sshclient.Target, event sshclient.CredentialEvent, _ bool) {
			observed = event
		},
	}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if observed != sshclient.CredentialTOTPUnavailable {
		t.Fatalf("credential observation = %q", observed)
	}
	if len(prompt.asked) != 1 || !strings.Contains(prompt.asked[0], "Verification code") {
		t.Fatalf("manual entry was not preserved: %#v", prompt.asked)
	}
}

func TestStoredPasswordAndTOTPAnswerACombinedChallenge(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{
		"Password: ": "hunter2", "OTP: ": "123456",
	}})
	prompt := &scriptedPrompter{}
	auth := sshclient.Auth{
		Password: func(sshclient.Target) (string, bool) { return "hunter2", true },
		TOTP: func(_ sshclient.Target, question string) (string, bool) {
			if question == "OTP: " {
				return "123456", true
			}
			return "", false
		},
	}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 0 {
		t.Fatalf("combined stored credentials still prompted: %#v", prompt.asked)
	}
}

func TestStoredTOTPDoesNotAnswerAnAmbiguousCodeQuestion(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{"Code: ": "manual"}})
	prompt := &scriptedPrompter{answers: []string{"manual"}}
	auth := sshclient.Auth{TOTP: func(sshclient.Target, string) (string, bool) {
		return "123456", true
	}}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 || !strings.Contains(prompt.secretly[0], "Code") {
		t.Fatalf("ambiguous prompt was not left to the user: %#v", prompt.secretly)
	}
}

func TestStoredTOTPDoesNotReplaceAPasswordForAnOTPNamedAccount(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{
		"Password for otp-admin: ": "hunter2",
	}})
	prompt := &scriptedPrompter{}
	totpAsked := false
	auth := sshclient.Auth{
		Password: func(sshclient.Target) (string, bool) { return "hunter2", true },
		TOTP: func(sshclient.Target, string) (string, bool) {
			totpAsked = true
			return "123456", true
		},
	}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if totpAsked {
		t.Fatal("the TOTP provider was called for an account name containing otp")
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
	auth := sshclient.Auth{Agent: unixSocketAgent(socket)}
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
	keyring, signer := agentKeyring(t)

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

// agentKeyring は、鍵をひとつ持つ agent の中身を作る。
func agentKeyring(t *testing.T) (agent.Agent, ssh.Signer) {
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
	return keyring, signer
}

// unixSocketAgent は、テストの agent が待つ unix socket を開く。どの OS でも unix
// socket として開くので、keys.NewAgent（Windows では固定の named pipe）は使わない。
func unixSocketAgent(socket string) sshclient.AgentConnector {
	return keys.Agent{
		Socket: func() string { return socket },
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", address)
		},
	}
}

// windowsAgentPipe は、Windows の OpenSSH agent が待つ named pipe の名前である。
// unix socket として開けば必ず失敗する宛先なので、sshclient が宛先を自分で
// 開かず、渡された開き方に従うことを確かめられる。
const windowsAgentPipe = `\\.\pipe\openssh-ssh-agent`

// pipeAgent は、named pipe の agent の代わりに、プロセス内の接続で鍵を貸す。
type pipeAgent struct {
	keyring agent.Agent
}

func (pipeAgent) Address() string { return windowsAgentPipe }

func (a pipeAgent) Connect(context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		_ = agent.ServeAgent(a.keyring, server)
		_ = server.Close()
	}()
	return client, nil
}

// unreachableAgent は、宛先はあるが開けない agent である。Windows で ssh-agent
// サービスが止まっているときと同じ形になる。
type unreachableAgent struct{}

func (unreachableAgent) Address() string { return windowsAgentPipe }

func (unreachableAgent) Connect(context.Context) (net.Conn, error) {
	return nil, errors.New("the agent pipe is not there")
}

// Windows の OpenSSH agent は SSH_AUTH_SOCK を設定せず named pipe で待つ。
// IdentityFile の無い接続でも、渡された agent の鍵で公開鍵認証が通る。
func TestAnAgentReachedThroughItsOwnTransportAuthenticatesWithoutAnIdentityFile(t *testing.T) {
	keyring, agentKey := agentKeyring(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{agentKey.PublicKey()}})

	auth := sshclient.Auth{Agent: pipeAgent{keyring: keyring}}
	if err := connect(t, server, targetWith(server), auth, &scriptedPrompter{}); err != nil {
		t.Fatalf("the agent's key did not connect: %v", err)
	}
}

// agent に届かなくても、ほかの認証方式は妨げない。Windows の ssh-agent サービスは
// 既定で無効なので、宛先があっても開けないことは普通に起きる。
func TestAnUnreachableAgentLetsPasswordAuthenticationProceed(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})

	auth := sshclient.Auth{
		Agent:    unreachableAgent{},
		Password: func(sshclient.Target) (string, bool) { return "hunter2", true },
	}
	if err := connect(t, server, targetWith(server), auth, &scriptedPrompter{}); err != nil {
		t.Fatalf("password authentication did not follow the unreachable agent: %v", err)
	}
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
	if !strings.Contains(prompt.secretly[0], "Saved password was rejected") {
		t.Fatalf("the fallback prompt hid why it asked again: %q", prompt.secretly[0])
	}
}

// 同じ問いで送ったばかりのパスワードは、まだ拒否されていない。残りの質問を
// 尋ねるときに「拒否された」と言えば、利用者は正しい保存値を書き換えかねない。
func TestAPasswordSentInTheSameChallengeIsNotCalledRejected(t *testing.T) {
	server := newTestServer(t, serverOptions{Keyboard: map[string]string{
		"Password: ": "hunter2", "Verification code: ": "123456", "Token PIN: ": "42",
	}})
	prompt := &scriptedPrompter{answers: []string{"42"}}
	auth := sshclient.Auth{
		Password: func(sshclient.Target) (string, bool) { return "hunter2", true },
		TOTP:     func(sshclient.Target, string) (string, bool) { return "123456", true },
	}

	if err := connect(t, server, targetWith(server), auth, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.secretly) != 1 || strings.Contains(prompt.secretly[0], "was rejected") {
		t.Fatalf("the remaining question claimed a rejection that did not happen: %#v", prompt.secretly)
	}
}

func TestKeyboardInteractiveTextIsShownWithoutControlSequences(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Keyboard:            map[string]string{"Code\x1b]0;owned\a: ": "42"},
		KeyboardName:        "site\x1b[2J",
		KeyboardInstruction: "type the code\x07",
	})
	prompt := &scriptedPrompter{answers: []string{"42"}}
	if err := connect(t, server, targetWith(server), sshclient.Auth{}, prompt); err != nil {
		t.Fatalf("connect = %v", err)
	}
	if len(prompt.asked) != 1 {
		t.Fatalf("asked = %#v", prompt.asked)
	}
	shown := prompt.asked[0]
	for _, forbidden := range []string{"\x1b", "\a", "\x07"} {
		if strings.Contains(shown, forbidden) {
			t.Fatalf("the prompt carried a control sequence: %q", shown)
		}
	}
	if !strings.Contains(shown, "site") || !strings.Contains(shown, "type the code") || !strings.Contains(shown, "Code") {
		t.Fatalf("the prompt lost the server's words: %q", shown)
	}
}
