package sshclient_test

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
)

func TestScanHostKeysCollectsWhatTheServerOffers(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "never used"})

	keys, err := sshclient.ScanHostKeys(context.Background(), nil, server.Address(), 5*time.Second)
	if err != nil {
		t.Fatalf("ScanHostKeys = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("keys = %d, want the one key this server has", len(keys))
	}
	if ssh.FingerprintSHA256(keys[0]) != ssh.FingerprintSHA256(server.HostKey.PublicKey()) {
		t.Errorf("the collected key is not the server's")
	}
}

// 鍵を集めるのに資格情報は要らない。サーバーは認証が届いた回数を数えて
// いるので、集めるだけのはずの操作が何かを差し出していないことを言える。
func TestScanHostKeysNeverAuthenticates(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "never offered"})

	keys, err := sshclient.ScanHostKeys(context.Background(), nil, server.Address(), 5*time.Second)
	if err != nil {
		t.Fatalf("ScanHostKeys = %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("no host key was collected")
	}
	if attempts := server.Attempts(); attempts != 0 {
		t.Fatalf("collecting a host key offered a credential %d time(s)", attempts)
	}
}

// サーバーが持たない種別は失敗ではない。持っている種別が取れていれば足りる。
func TestScanHostKeysSkipsTheTypesTheServerDoesNotHave(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "never offered"})
	// ed25519 の鍵しか無いサーバーである。ScanAlgorithms は 7 種別を尋ねる。
	keys, err := sshclient.ScanHostKeys(context.Background(), nil, server.Address(), 5*time.Second)
	if err != nil {
		t.Fatalf("ScanHostKeys = %v", err)
	}
	if len(keys) != 1 || keys[0].Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("keys = %#v", keys)
	}
}

// 一つも取れなかったときだけ理由を返す。
func TestScanHostKeysReportsAnUnreachableAddress(t *testing.T) {
	// 誰も待ち受けていないポート。接続は即座に拒否される。
	if _, err := sshclient.ScanHostKeys(
		context.Background(), nil, "127.0.0.1:1", time.Second,
	); err == nil {
		t.Fatal("an unreachable address reported no error")
	}
}
