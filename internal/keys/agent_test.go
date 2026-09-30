package keys_test

import (
	"context"
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
	"sshc/internal/platform"
)

// runAgent は、プロセス内の ssh-agent を unix ソケットで待ち受けさせる。
//
// 本物のプロトコルである。検査対象は agent との通信方式そのものなので、
// 模したものと突き合わせても何も確かめられない。
func runAgent(t *testing.T) (string, agent.Agent) {
	t.Helper()
	keyring := agent.NewKeyring()

	// t.TempDir() は使わない。unix ソケットのパスには 100 バイト程度の
	// 上限があり、テスト名を含むあの長いパスは macOS でそれを超える。超えると
	// bind が失敗し、この検査は skip として静かに消える。
	directory, err := os.MkdirTemp("", "sshc-agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })

	socket := filepath.Join(directory, "s")
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
	return socket, keyring
}

// agentKey は、鍵ひとつの秘密鍵と公開鍵のファイルの中身を返す。agent の adapter は
// ファイルに触れないので、ディスクには書かない。
func agentKey(t *testing.T, passphrase []byte) (private, public []byte) {
	t.Helper()
	key, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	private, err = keys.EncodePrivateKey(key, "fixture@sshc", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	public, err = keys.EncodePublicKey(key, "fixture@sshc")
	if err != nil {
		t.Fatal(err)
	}
	return private, public
}

// agentFor は、このテスト用 agent へ接続する adapter を組み立てる。
//
// NewAgent を通さない。あれが決めるのは「この OS の agent はどこに居るか」
// であり、それは OS ごとに違う。Windows では固定の named pipe を指し、
// SSH_AUTH_SOCK を読まない。ここで確かめたいのは宛先の決め方ではなく、
// 繋がったあとのプロトコルの通信方式なので、宛先はテストが直接与える。
// unix ソケットは Windows 10 以降も使えるので、この管はどのホストでも通る。
func agentFor(socket string) keys.Agent {
	return keys.Agent{
		Socket: func() string { return socket },
		Dial: func(ctx context.Context, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", address)
		},
	}
}

func TestAgentAddsListsAndRemovesThroughTheRealProtocol(t *testing.T) {
	socket, keyring := runAgent(t)
	private, public := agentKey(t, nil)
	adapter := agentFor(socket)

	if !adapter.Available(context.Background()) {
		t.Fatal("a reachable agent reported itself unavailable")
	}
	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKey: private, Comment: "/home/fixture/.ssh/id_ed25519",
	}); err != nil {
		t.Fatalf("Add = %v", err)
	}

	loaded, err := keyring.List()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("the agent holds %d key(s): %v", len(loaded), err)
	}
	if loaded[0].Comment != "/home/fixture/.ssh/id_ed25519" {
		t.Errorf("the agent names the key %q, want the requested comment", loaded[0].Comment)
	}

	identities, err := adapter.List(context.Background())
	if err != nil {
		t.Fatalf("List = %v", err)
	}
	if len(identities) != 1 || identities[0].Algorithm != ssh.KeyAlgoED25519 {
		t.Fatalf("identities = %#v", identities)
	}
	if identities[0].Fingerprint == "" || identities[0].Bits != 256 {
		t.Errorf("identity = %#v", identities[0])
	}

	if err := adapter.Remove(context.Background(), public); err != nil {
		t.Fatalf("Remove = %v", err)
	}
	if loaded, err := keyring.List(); err != nil || len(loaded) != 0 {
		t.Fatalf("the agent still holds %d key(s): %v", len(loaded), err)
	}
}

// 鍵の復号はこのプロセスで行う。パスフレーズが agent へ渡ることはない。
func TestAgentDecryptsTheKeyBeforeHandingItOver(t *testing.T) {
	socket, keyring := runAgent(t)
	private, _ := agentKey(t, []byte("correct horse"))
	adapter := agentFor(socket)

	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKey: private, Passphrase: []byte("wrong"),
	}); !errors.Is(err, keys.ErrWrongPassphrase) {
		t.Fatalf("Add with a wrong passphrase = %v, want ErrWrongPassphrase", err)
	}
	if loaded, _ := keyring.List(); len(loaded) != 0 {
		t.Fatal("a wrong passphrase still put a key into the agent")
	}

	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKey: private, Passphrase: []byte("correct horse"),
	}); err != nil {
		t.Fatalf("Add = %v", err)
	}
	if loaded, _ := keyring.List(); len(loaded) != 1 {
		t.Fatal("the decrypted key did not reach the agent")
	}
}

// 変数があることは、その先に誰かがいることを意味しない。死んだ端末が残した
// SSH_AUTH_SOCK は、いつまでも残る。
func TestAgentReportsAnUnreachableSocketAsUnavailable(t *testing.T) {
	adapter := agentFor(filepath.Join(t.TempDir(), "nobody-is-here.sock"))

	if adapter.Available(context.Background()) {
		t.Fatal("a socket nobody listens on reported itself available")
	}
	if _, err := adapter.List(context.Background()); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("List = %v, want ErrAgentUnavailable", err)
	}
}

// 開けなかった理由は、agent 転送の失敗としてターミナルへそのまま出る。改行を含むと、
// 次の行が行頭へ戻らない。
func TestAgentExplainsAnUnreachableSocketOnOneLine(t *testing.T) {
	adapter := agentFor(filepath.Join(t.TempDir(), "nobody-is-here.sock"))

	_, err := adapter.Connect(context.Background())
	if !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("Connect = %v, want ErrAgentUnavailable", err)
	}
	if strings.ContainsAny(err.Error(), "\r\n") {
		t.Fatalf("the reason spans several lines: %q", err.Error())
	}
}

// deadlineRecordingConn は、SetDeadline が呼ばれたかを覚える接続である。
type deadlineRecordingConn struct {
	net.Conn
	deadlineSet bool
}

func (conn *deadlineRecordingConn) SetDeadline(deadline time.Time) error {
	conn.deadlineSet = true
	return conn.Conn.SetDeadline(deadline)
}

// agentOver は、開くと必ず conn を返す agent である。
func agentOver(conn net.Conn) keys.Agent {
	return keys.Agent{
		Socket: func() string { return "recorded" },
		Dial:   func(context.Context, string) (net.Conn, error) { return conn, nil },
	}
}

// 署名の要求は、接続の認証や agent 転送のあいだのいつ来るか分からない。Connect が
// 開いた接続に期限を付けると、転送した agent は Timeout の後に切れる。
func TestConnectLeavesTheOpenedAgentWithoutADeadline(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	recorded := &deadlineRecordingConn{Conn: client}

	conn, err := agentOver(recorded).Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	defer func() { _ = conn.Close() }()
	if recorded.deadlineSet {
		t.Fatal("Connect put a deadline on the agent connection")
	}
}

// Keys 画面の一往復は、応答しない agent で止まらないよう、開いた接続にも期限を付ける。
func TestTheKeysScreenBoundsEachAgentRoundTripWithADeadline(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = server.Close() }()
	recorded := &deadlineRecordingConn{Conn: client}

	if !agentOver(recorded).Available(context.Background()) {
		t.Fatal("an agent that opens reported itself unavailable")
	}
	if !recorded.deadlineSet {
		t.Fatal("the Keys screen opened the agent without a deadline")
	}
}

func TestAgentWithoutASocketIsUnavailable(t *testing.T) {
	adapter := agentFor("")
	private, _ := agentKey(t, nil)

	if adapter.Available(context.Background()) {
		t.Fatal("an agent with no socket reported itself available")
	}
	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKey: private,
	}); !errors.Is(err, platform.ErrAgentUnavailable) {
		t.Fatalf("Add without a socket = %v, want ErrAgentUnavailable", err)
	}
}

// 寿命つきの登録は agent が受け取る。
func TestAgentPassesTheRequestedLifetime(t *testing.T) {
	socket, keyring := runAgent(t)
	private, _ := agentKey(t, nil)

	if err := agentFor(socket).Add(context.Background(), platform.AgentAddRequest{
		PrivateKey: private, LifetimeSeconds: 60,
	}); err != nil {
		t.Fatalf("Add = %v", err)
	}
	if loaded, _ := keyring.List(); len(loaded) != 1 {
		t.Fatal("a key with a lifetime did not reach the agent")
	}
}
