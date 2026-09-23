//go:build linux

package vpn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
)

// 本物のコンテナと本物のトンネルで、経路が成立することを確かめる。
//
// Docker のイメージ作成に apt が要るので、既定では走らせない。CI の他の検査が
// 外部のミラーの機嫌で落ちるようにしないためである。手元で確かめるときは
// SSHC_VPN_DOCKER_TEST=1 を付ける。

const (
	// tunnelServerAddress は、テスト用の相手がトンネル側で名乗るアドレスである。
	tunnelServerAddress = "10.77.0.1"
	// tunnelClientAddress は、engine 側のコンテナがトンネル側で名乗るアドレスである。
	tunnelClientAddress = "10.77.0.2/32"
	// echoPort は、テスト用の相手が待ち受けるポートである。
	echoPort = 2222
	// dockerTestTimeout は、イメージ作成を含む一連の操作の上限である。
	dockerTestTimeout = 6 * time.Minute
)

func requireDockerTest(t *testing.T) (*Manager, context.Context) {
	t.Helper()
	if os.Getenv("SSHC_VPN_DOCKER_TEST") != "1" {
		t.Skip("SSHC_VPN_DOCKER_TEST=1 のときだけ、本物のコンテナで確かめる")
	}
	ctx, cancel := context.WithTimeout(context.Background(), dockerTestTimeout)
	t.Cleanup(cancel)
	manager := New(t.TempDir(), os.Getuid())
	if err := manager.Available(ctx); err != nil {
		t.Skipf("この機械ではVPN経路を作れない: %v", err)
	}
	return manager, ctx
}

// keyPair は、wireguard の鍵ひとつぶんを作る。
func keyPair(t *testing.T) (private, public string) {
	t.Helper()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal(err)
	}
	// Curve25519 の秘密鍵の形に整える。wg genkey と同じ処理である。
	secret[0] &= 248
	secret[31] &= 127
	secret[31] |= 64
	shared, err := curve25519.X25519(secret[:], curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(secret[:]), base64.StdEncoding.EncodeToString(shared)
}

// startTunnelPeer は、トンネルの相手を1台立てる。
//
// 同じイメージを使うが、entrypoint を差し替えて、待ち受ける側の wireguard と
// 返事をするだけのサービスを動かす。返すのは、その相手の公開鍵と、Dockerの
// 通常回線で届くアドレスである。
func startTunnelPeer(t *testing.T, manager *Manager, ctx context.Context, image, clientPublicKey string) (publicKey, bridgeAddress string) {
	t.Helper()
	private, public := keyPair(t)
	name := fmt.Sprintf("sshc-vpn-test-peer-%d", os.Getpid())
	configuration := strings.Join([]string{
		"[Interface]",
		"PrivateKey = " + private,
		"ListenPort = 51820",
		"",
		"[Peer]",
		"PublicKey = " + clientPublicKey,
		"AllowedIPs = " + tunnelClientAddress,
		"",
	}, "\n")
	script := strings.Join([]string{
		"set -eu",
		"umask 077",
		"mkdir -p /run/peer",
		// 本番のagentと同じく、設定が届くのを待つ。--detach で起動するので
		// 標準入力からは読めない。
		"seconds=0",
		"while [ ! -s /run/peer/wg.conf ]; do",
		"  if [ \"$seconds\" -ge 30 ]; then echo '設定が届かなかった' >&2; exit 1; fi",
		"  sleep 1; seconds=$((seconds + 1))",
		"done",
		"wireguard-go wg0",
		"wg setconf wg0 /run/peer/wg.conf",
		"ip address add " + tunnelServerAddress + "/24 dev wg0",
		"ip link set wg0 up",
		// トンネル側とDockerの通常回線側の両方で待ち受ける。素の回線から
		// 届いてしまわないことも確かめたいからである。
		fmt.Sprintf("exec socat TCP-LISTEN:%d,fork,reuseaddr SYSTEM:'echo tunnelled'", echoPort),
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--device", "/dev/net/tun", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("トンネルの相手を起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.output(context.Background(), "logs", "--tail", "40", name)
			t.Logf("トンネルの相手のログ:\n%s", logs)
		}
		_, _ = manager.docker.output(context.Background(), "rm", "--force", name)
	})
	if _, err := manager.docker.outputWithInput(ctx, configuration, "exec", "-i", name,
		"sh", "-c", "cat > /run/peer/wg.conf"); err != nil {
		t.Fatalf("相手へ設定を渡せない: %v", err)
	}
	address, err := manager.docker.output(ctx, "container", "inspect", "--format",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatal(err)
	}
	return public, strings.TrimSpace(address)
}

// トンネルの中にいる相手へ、engine が中継越しに繋げる。
func TestAConnectionReachesTheTargetThroughTheTunnel(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)

	profile := Profile{
		Name:    "e2e",
		Backend: WireGuard,
		Target:  Endpoint{Host: tunnelServerAddress, Port: echoPort},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuardPrivateKey: clientPrivate}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	connection, err := manager.Dial(ctx, profile, secrets)
	if err != nil {
		t.Fatalf("Dial = %v", err)
	}
	defer func() { _ = connection.Close() }()

	_ = connection.SetReadDeadline(time.Now().Add(20 * time.Second))
	answer, err := io.ReadAll(connection)
	if err != nil && len(answer) == 0 {
		t.Fatalf("トンネル越しに読めない: %v", err)
	}
	if !strings.Contains(string(answer), "tunnelled") {
		t.Fatalf("相手からの返事 = %q", answer)
	}

	status, err := manager.Status(ctx, profile.Name)
	if err != nil || !status.Running || status.RelaySocket == "" {
		t.Fatalf("Status = %+v, %v", status, err)
	}
}

// トンネルが成立しないとき、接続先へはDockerの通常回線からも届かない。
func TestTheTargetIsUnreachableWhileTheTunnelIsNotUp(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	_, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	// 相手の公開鍵として、相手が持っていない鍵を渡す。握手は成立しない。
	_, strangerPublic := keyPair(t)

	profile := Profile{
		Name:    "e2e-closed",
		Backend: WireGuard,
		// 接続先は、Dockerの通常回線からも届くアドレスである。トンネルが
		// 成立していないあいだ、そちらへ落ちないことを確かめる。
		Target: Endpoint{Host: peerAddress, Port: echoPort},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: strangerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuardPrivateKey: clientPrivate}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	connection, err := manager.Dial(ctx, profile, secrets)
	if err != nil {
		// 中継が開く前に諦めた場合も、通常回線へは落ちていない。
		return
	}
	defer func() { _ = connection.Close() }()

	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	answer := make([]byte, 32)
	read, err := connection.Read(answer)
	if err == nil && read > 0 && strings.Contains(string(answer[:read]), "tunnelled") {
		t.Fatalf("トンネルが無いまま接続先へ届いた: %q", answer[:read])
	}
}

// 前回のengineが残したコンテナは、引き継がずに止める。
//
// どの設定で経路を張ったのかを確かめられないものを使い続けるより、止めて
// 作り直す方が安全である。
func TestSessionsLeftByAPreviousEngineAreDiscarded(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	profile := Profile{
		Name:    "orphan",
		Backend: WireGuard,
		Target:  Endpoint{Host: tunnelServerAddress, Port: echoPort},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })
	if err := manager.Start(ctx, profile, Secrets{WireGuardPrivateKey: clientPrivate}); err != nil {
		t.Fatalf("Start = %v", err)
	}

	// engineが起動し直された状況。前回のセッションのことは何も覚えていない。
	restarted := New(t.TempDir(), os.Getuid())
	if err := restarted.DiscardOrphans(ctx); err != nil {
		t.Fatalf("DiscardOrphans = %v", err)
	}

	status, err := manager.Status(ctx, profile.Name)
	if err != nil {
		t.Fatalf("Status = %v", err)
	}
	if status.Running {
		t.Fatal("前回のengineのコンテナが動いたまま残った")
	}
}

// L2TP/IPsec の枝も、設定の書き出しからトンネルの開始までを実際に走らせる。
//
// 本物のVPN装置は用意しない。届かない相手に対して、設定の解釈・経路の準備・
// strongSwan の起動まで進み、そこで理由を添えて失敗することを確かめる。ここで
// 止まらなければ、jq の読み出し、ファイルの書き出し、名前の解決、iptables、
// ipsec の起動までは動いている。
func TestTheL2TPBranchRunsUntilTheServerRefusesIt(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	if err := requireTunnelDevice(L2TPIPsec); err != nil {
		t.Skipf("この機械では l2tp を試せない: %v", err)
	}
	profile := Profile{
		Name:    "l2tp-unreachable",
		Backend: L2TPIPsec,
		Target:  Endpoint{Host: "10.77.1.1", Port: 22},
		L2TP: &L2TPSettings{
			// TEST-NET-1。誰も応答しない。
			Server:   "192.0.2.1",
			Username: "fixture",
		},
	}
	secrets := Secrets{L2TPPassword: "fixture-password", IPsecPSK: "fixture-psk"}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	err := manager.Start(ctx, profile, secrets)

	if err == nil {
		t.Fatal("届かない相手に対して経路が成立した")
	}
	if !strings.Contains(err.Error(), "IPsec") {
		t.Fatalf("失敗の理由が IPsec の段階を指していない: %v", err)
	}
	// 秘密は、利用者へ見せる失敗の文面に現れない。
	for _, forbidden := range []string{secrets.L2TPPassword, secrets.IPsecPSK} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("失敗の文面に秘密が現れた: %v", err)
		}
	}
}

// 誰も通っていない経路は畳み、通っている経路は残す。
func TestAnIdleRouteIsStoppedAndAUsedOneIsKept(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	profile := Profile{
		Name:    "idle",
		Backend: WireGuard,
		Target:  Endpoint{Host: tunnelServerAddress, Port: echoPort},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	// 時計を手で進める。閉じた時刻と掃除の時刻が同じでは、無操作の長さを
	// 測れない。
	clock := time.Now()
	manager.now = func() time.Time { return clock }

	connection, err := manager.Dial(ctx, profile, Secrets{WireGuardPrivateKey: clientPrivate})
	if err != nil {
		t.Fatalf("Dial = %v", err)
	}

	// 接続が開いているあいだは、どれだけ経っても畳まない。
	clock = clock.Add(time.Hour)
	manager.StopIdle(ctx, time.Minute)
	if status, err := manager.Status(ctx, profile.Name); err != nil || !status.Running {
		t.Fatalf("通っている接続があるのに畳んだ: %+v, %v", status, err)
	}

	// 閉じてから時間が経てば畳む。
	_ = connection.Close()
	clock = clock.Add(time.Hour)
	manager.StopIdle(ctx, time.Minute)

	status, err := manager.Status(ctx, profile.Name)
	if err != nil {
		t.Fatalf("Status = %v", err)
	}
	if status.Running {
		t.Fatal("誰も通っていない経路が残った")
	}
}
