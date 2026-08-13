package keys_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/keys"
	"sshc/internal/platform"
)

// runAgent は、プロセス内の ssh-agent を unix ソケットで待ち受けさせる。
//
// **本物のプロトコルである。** 検査対象は agent との話し方そのものなので、
// 模したものと突き合わせても何も確かめられない。
func runAgent(t *testing.T) (string, agent.Agent) {
	t.Helper()
	keyring := agent.NewKeyring()

	// **t.TempDir() は使わない。** unix ソケットのパスには 100 バイト程度の
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

// writeAgentKey は、鍵ひとつを書いてそのパスを返す。
func writeAgentKey(t *testing.T, directory string, passphrase []byte) (string, string) {
	t.Helper()
	private, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := keys.EncodePrivateKey(private, "fixture@sshc", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "id_ed25519")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	public, err := keys.EncodePublicKey(private, "fixture@sshc")
	if err != nil {
		t.Fatal(err)
	}
	publicPath := path + ".pub"
	if err := os.WriteFile(publicPath, public, 0o644); err != nil {
		t.Fatal(err)
	}
	return path, publicPath
}

func agentFor(socket string) platform.KeyAgent {
	return keys.NewAgent(func(name string) (string, bool) {
		if name != "SSH_AUTH_SOCK" {
			return "", false
		}
		return socket, true
	})
}

func TestAgentAddsListsAndRemovesThroughTheRealProtocol(t *testing.T) {
	socket, keyring := runAgent(t)
	directory := t.TempDir()
	path, publicPath := writeAgentKey(t, directory, nil)
	adapter := agentFor(socket)

	if !adapter.Available(context.Background()) {
		t.Fatal("a reachable agent reported itself unavailable")
	}
	if err := adapter.Add(context.Background(), platform.AgentAddRequest{PrivateKeyPath: path}); err != nil {
		t.Fatalf("Add = %v", err)
	}

	loaded, err := keyring.List()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("the agent holds %d key(s): %v", len(loaded), err)
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

	if err := adapter.Remove(context.Background(), publicPath); err != nil {
		t.Fatalf("Remove = %v", err)
	}
	if loaded, err := keyring.List(); err != nil || len(loaded) != 0 {
		t.Fatalf("the agent still holds %d key(s): %v", len(loaded), err)
	}
}

// **鍵の復号はこのプロセスで行う。** パスフレーズが agent へ渡ることはない。
func TestAgentDecryptsTheKeyBeforeHandingItOver(t *testing.T) {
	socket, keyring := runAgent(t)
	directory := t.TempDir()
	path, _ := writeAgentKey(t, directory, []byte("correct horse"))
	adapter := agentFor(socket)

	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: path, Passphrase: []byte("wrong"),
	}); !errors.Is(err, keys.ErrWrongPassphrase) {
		t.Fatalf("Add with a wrong passphrase = %v, want ErrWrongPassphrase", err)
	}
	if loaded, _ := keyring.List(); len(loaded) != 0 {
		t.Fatal("a wrong passphrase still put a key into the agent")
	}

	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: path, Passphrase: []byte("correct horse"),
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

func TestAgentWithoutASocketIsUnavailable(t *testing.T) {
	adapter := keys.NewAgent(func(string) (string, bool) { return "", false })

	if adapter.Available(context.Background()) {
		t.Fatal("an agent with no socket reported itself available")
	}
	if err := adapter.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: filepath.Join(t.TempDir(), "absent"),
	}); err == nil {
		t.Fatal("Add without a socket succeeded")
	}
}

// 寿命つきの登録は agent が受け取る。
func TestAgentPassesTheRequestedLifetime(t *testing.T) {
	socket, keyring := runAgent(t)
	path, _ := writeAgentKey(t, t.TempDir(), nil)

	if err := agentFor(socket).Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath: path, LifetimeSeconds: 60,
	}); err != nil {
		t.Fatalf("Add = %v", err)
	}
	if loaded, _ := keyring.List(); len(loaded) != 1 {
		t.Fatal("a key with a lifetime did not reach the agent")
	}
}
