package sshclient_test

import (
	"context"
	"crypto/rand"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/keys"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// 接続ログの中身。どの深さで何を言い、何を言わないか。

// passphraseKeyPair は、パスフレーズ付きの鍵ひとつをメモリ上に作る。
func passphraseKeyPair(t *testing.T, passphrase string) (contents []byte, public ssh.PublicKey) {
	t.Helper()
	private, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := keys.EncodePrivateKey(private, "fixture", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, signer.PublicKey()
}

func openWithLog(t *testing.T, dialer sshclient.Dialer, target sshclient.Target, level sshclient.Verbosity) terminal.Process {
	t.Helper()
	dialer.Verbosity = func() sshclient.Verbosity { return level }
	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	return process
}

func expectLines(t *testing.T, seen string, wanted ...string) {
	t.Helper()
	for _, want := range wanted {
		if !strings.Contains(seen, want) {
			t.Errorf("the connection log did not contain %q:\n%s", want, seen)
		}
	}
}

// 鍵が複数あるとき、読めない鍵は他の鍵で通れば誰にも報告されない。深さ 2 は
// 鍵ごとに指紋と復号の仕方を言い、使えなかった鍵も言う。パスフレーズは言わない。
func TestTheDetailedLogNamesEachKeyByFingerprintAndHowItWasUnlocked(t *testing.T) {
	contents, public := passphraseKeyPair(t, "correct horse")
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	dialer := dialerFor(t, server, sshclient.Auth{
		ReadFile: func(path string) ([]byte, error) {
			if path == "/keys/missing" {
				return nil, io.ErrUnexpectedEOF
			}
			return contents, nil
		},
		Stored: func(string) (string, bool) { return "correct horse", true },
	})
	process := openWithLog(t, dialer, targetWith(server, "/keys/id_ed25519", "/keys/missing"), sshclient.Detailed)

	seen := readUntil(t, process, "ready")
	expectLines(t, seen,
		"認証方式の候補（試す順）：publickey, keyboard-interactive, password",
		"鍵 /keys/id_ed25519："+public.Type()+" "+ssh.FingerprintSHA256(public)+"（保存済みパスフレーズで復号）",
		"鍵 /keys/missing は使えません：unexpected EOF",
		"公開鍵認証で試す鍵：1 件",
		"認証方式 publickey で認証されました。",
	)
	if strings.Contains(seen, "correct horse") {
		t.Fatalf("the connection log exposed the passphrase:\n%s", seen)
	}
}

// 「一致しない鍵」と断られたユーザーが見たいのは、どの行のどの鍵と比べたかである。
func TestTheDetailedLogPointsAtTheKnownHostsLineItComparedWith(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	hostField := "[" + server.Host() + "]:" + server.Port()

	t.Run("matched", func(t *testing.T) {
		known := "# written by hand\n" + knownHostsLine(hostField, server.HostKey.PublicKey())
		dialer := sshclient.Dialer{Auth: auth, HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return []byte(known), nil },
		}}
		process := openWithLog(t, dialer, targetWith(server, path), sshclient.Detailed)
		expectLines(t, readUntil(t, process, "ready"),
			"サーバーのホスト鍵："+server.HostKey.PublicKey().Type()+" "+ssh.FingerprintSHA256(server.HostKey.PublicKey()),
			"ホスト鍵は known_hosts の 2 行目と一致しました。",
		)
	})

	t.Run("changed", func(t *testing.T) {
		_, _, another := keyPair(t)
		known := "# written by hand\n" + knownHostsLine(hostField, another)
		dialer := sshclient.Dialer{Auth: auth, HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return []byte(known), nil },
		}}
		process := openWithLog(t, dialer, targetWith(server, path), sshclient.Detailed)
		expectLines(t, readUntil(t, process, "ホスト鍵を受け入れませんでした"),
			"known_hosts の 2 行目には別の鍵があります："+another.Type()+" "+ssh.FingerprintSHA256(another),
			"ホスト鍵を受け入れませんでした：the host key does not match the one in known_hosts",
		)
	})
}

// サーバーの banner は、ユーザーへ向けた案内である。深さ 1 から出し、
// サーバーが書いた文なので制御文字は落とす。既定の無言では出さない。
func TestTheBannerIsShownFromBriefUpwardsWithoutControlCharacters(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		Banner:     "Welcome\x07 to the fixture\nAuthorised users only\n",
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	dialer := dialerFor(t, server, sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }})

	process := openWithLog(t, dialer, targetWith(server, path), sshclient.Brief)
	seen := readUntil(t, process, "ready")
	expectLines(t, seen, "サーバーからの案内：", "[sshc][debug1]   Welcome to the fixture", "[sshc][debug1]   Authorised users only")
	if strings.Contains(seen, "\x07") {
		t.Fatalf("the banner carried a control character to the terminal:\n%q", seen)
	}

	quiet := openWithLog(t, dialer, targetWith(server, path), sshclient.Quiet)
	if seen := readUntil(t, quiet, "ready"); strings.Contains(seen, "Welcome") {
		t.Fatalf("the banner was shown although the log was not asked for:\n%s", seen)
	}
}

// セッションを開くときに何を頼んだかは、深さ 2 で分かる。効かない設定も
// ここで言う。`sshc info` を開かずに、繋いだ端末でそのまま読めるためである。
func TestTheDetailedLogListsWhatTheSessionAskedFor(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	dialer := dialerFor(t, server, sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }})
	target := targetWith(server, path)
	target.SetEnv = []sshclient.EnvVar{{Name: "LANG", Value: "ja_JP.UTF-8"}}
	target.KeepAlive = 15 * time.Second
	target.KeepAliveMax = 2
	target.Notices = []sshclient.Notice{{Keyword: "RemoteForward", Detail: "sshc does not ask the remote to listen"}}

	process := openWithLog(t, dialer, target, sshclient.Detailed)
	seen := readUntil(t, process, "keepalive")
	expectLines(t, seen,
		"設定 RemoteForward は適用しません：sshc does not ask the remote to listen",
		"環境変数 LANG を送りました。",
		"端末を要求します：120 列 × 40 行（TERM=xterm-256color）。",
		"シェルを起動します。",
		"keepalive：15s ごとに送り、2 回続けて応答が無ければ切断します。",
	)
	if strings.Contains(seen, "ja_JP.UTF-8") {
		t.Fatalf("the connection log wrote an environment value:\n%s", seen)
	}
}

// 深さ 3 は agent の鍵を指紋で一覧する。どの鍵が agent から来たかは、
// 他に確かめる場所が無い。
func TestTheFullLogListsAgentKeysByFingerprint(t *testing.T) {
	socket, agentKey := runTestAgent(t, t.TempDir())
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{agentKey.PublicKey()},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	dialer := dialerFor(t, server, sshclient.Auth{AgentSocket: socket})

	process := openWithLog(t, dialer, targetWith(server), sshclient.Full)
	expectLines(t, readUntil(t, process, "ready"),
		"agent の鍵：1 件（"+socket+"）",
		"agent の鍵："+agentKey.PublicKey().Type()+" "+ssh.FingerprintSHA256(agentKey.PublicKey()),
		"名乗るホスト鍵アルゴリズム：ssh-ed25519",
		"サーバーの SSH バージョン：SSH-2.0-Go",
	)
}

// パスワードは出どころだけを言う。保存済みなら送ったことを、無ければ尋ねる
// ことを言い、値そのものは書かない。
func TestTheDetailedLogSaysWhereThePasswordCameFrom(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Password: "hunter2",
		OnShell:  func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	dialer := dialerFor(t, server, sshclient.Auth{
		Password: func(sshclient.Target) (string, bool) { return "hunter2", true },
	})

	process := openWithLog(t, dialer, targetWith(server), sshclient.Detailed)
	seen := readUntil(t, process, "ready")
	expectLines(t, seen,
		"publickey は試しません：IdentityFile が無く、agent も使いません。",
		"認証方式を試します：password",
		"保存済みパスワードを送ります。",
		"認証方式 password で認証されました。",
	)
	if strings.Contains(seen, "hunter2") {
		t.Fatalf("the connection log exposed the password:\n%s", seen)
	}
}
