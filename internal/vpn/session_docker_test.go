//go:build linux

package vpn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"

	"sshc/internal/connectionlog"
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
	// secondEchoPort は、同じ経路で2つ目の接続先として使うポートである。
	secondEchoPort = 2223
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
	manager := New(t.TempDir(), os.Getuid(), nil)
	if err := manager.Available(ctx); err != nil {
		t.Skipf("このマシンではVPN経路を作れない: %v", err)
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
		fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr SYSTEM:'echo second' &", secondEchoPort),
		fmt.Sprintf("exec socat TCP-LISTEN:%d,fork,reuseaddr SYSTEM:'echo tunnelled'", echoPort),
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--device", "/dev/net/tun", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("トンネルの相手を起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.combined(context.Background(), "logs", "--tail", "40", name)
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
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	// 接続ログ（-vvv）には、経路の準備と接続先への接続の各段階が出る。
	var connectionLog attemptRecord
	requireAnswer(t, manager, connectionlog.With(ctx, &connectionLog), dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort)}, "tunnelled")
	for _, want := range []string{
		"VPN経路 e2e を起動します（wireguard）。",
		"docker：",
		"VPNに接続しました（wireguard、インターフェース wg0",
		"コンテナの中継：接続先 " + tunnelServerAddress + " への経路とパケットフィルタを追加しました",
		fmt.Sprintf("VPN経由で %s:%d に接続しました", tunnelServerAddress, echoPort),
		"[debug3] docker exec --interactive",
	} {
		if !strings.Contains(connectionLog.text(), want) {
			t.Fatalf("接続ログに %q が無い:\n%s", want, connectionLog.text())
		}
	}
	// 同じ内容は、経路ごとの記録にも残り、ログとして読める。
	if logs, err := manager.Logs(ctx, profile.Name, secrets); err != nil || !strings.Contains(logs, "VPNに接続しました") {
		t.Fatalf("Logs = %v\n%s", err, logs)
	}
	// 同じ経路で、別の接続先へも届く。接続先はプロファイルではなく接続が決める。
	requireAnswer(t, manager, ctx, dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", tunnelServerAddress, secondEchoPort)}, "second")

	status, err := manager.Status(ctx, profile.Name)
	if err != nil || !status.Running || status.RelaySocket == "" {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	if status.Phase != "" {
		t.Fatalf("経路が立ったあとも用意中のままだった: %q", status.Phase)
	}
}

// dialTarget は、経路ひとつと、その先で繋ぐ接続先である。
type dialTarget struct {
	profile Profile
	secrets Secrets
	address string
}

// requireAnswer は、経路を通して接続先へ繋ぎ、相手の返事に want が含まれることを確かめる。
func requireAnswer(t *testing.T, manager *Manager, ctx context.Context, target dialTarget, want string) {
	t.Helper()
	connection, err := manager.Dial(ctx, target.profile, target.secrets, target.address)
	if err != nil {
		t.Fatalf("Dial(%s) = %v", target.address, err)
	}
	defer func() { _ = connection.Close() }()
	_ = connection.SetReadDeadline(time.Now().Add(20 * time.Second))
	answer, err := io.ReadAll(connection)
	if err != nil && len(answer) == 0 {
		t.Fatalf("トンネル越しに読めない（%s）: %v", target.address, err)
	}
	if !strings.Contains(string(answer), want) {
		t.Fatalf("%s の返事 = %q, want %q", target.address, answer, want)
	}
}

// VPNサーバーそのものは接続先にできない。その経路は、トンネルの外側の通信を
// トンネルの中へ曲げてしまう。
func TestTheVPNServerItselfIsRefusedAsADestination(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	profile := Profile{
		Name:    "e2e-server",
		Backend: WireGuard,
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	connection, err := manager.Dial(ctx, profile, secrets, fmt.Sprintf("%s:%d", peerAddress, echoPort))
	if err == nil {
		_ = connection.Close()
		t.Fatal("VPNサーバーそのものへの経路を作った")
	}
	var failure *TargetFailure
	if !errors.As(err, &failure) || failure.Reason != FailureTargetIsServer {
		t.Fatalf("Dial = %v, want %s", err, FailureTargetIsServer)
	}
	// 断ったあとも、トンネルは使える。
	requireAnswer(t, manager, ctx, dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort)}, "tunnelled")
}

// 接続先へ届かない理由は、名前解決の失敗と、接続の失敗を分けて返す。
func TestAnUnreachableDestinationSaysWhy(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	profile := Profile{
		Name:    "e2e-unreachable",
		Backend: WireGuard,
		// トンネルの相手を DNS サーバーとして書く。相手は DNS に答えないので、
		// 名前解決は必ず失敗する。
		DNS: []string{tunnelServerAddress},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	for _, test := range []struct {
		address string
		want    FailureReason
	}{
		{"missing.invalid:22", FailureTargetUnresolved},
		// 相手はこのポートで待ち受けていない。
		{fmt.Sprintf("%s:%d", tunnelServerAddress, 2299), FailureTargetUnreachable},
	} {
		connection, err := manager.Dial(ctx, profile, secrets, test.address)
		if err == nil {
			_ = connection.Close()
			t.Fatalf("Dial(%s) succeeded", test.address)
		}
		var failure *TargetFailure
		if !errors.As(err, &failure) || failure.Reason != test.want {
			t.Fatalf("Dial(%s) = %v, want %s", test.address, err, test.want)
		}
	}
	// 断った接続は数えない。
	if open := openConnections(manager.state(profile.Name)); open != 0 {
		t.Fatalf("断った接続を数えたまま = %d", open)
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
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: strangerPublic,
			Address:       tunnelClientAddress,
		},
	}
	secrets := Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	// 接続先は、Dockerの通常回線からも届くアドレスである。トンネルが成立して
	// いないあいだ、そちらへ落ちないことを確かめる。
	connection, err := manager.Dial(ctx, profile, secrets, fmt.Sprintf("%s:%d", peerAddress, echoPort))
	if err == nil {
		_ = connection.Close()
		t.Fatal("相手とハンドシェイクできないまま、経路が用意できたことになった")
	}
	// 握手できないことを、理由として返す。鍵かサーバーの誤りを利用者が疑える。
	var failure *SessionFailure
	if !errors.As(err, &failure) || failure.Reason != FailureHandshakeTimeout {
		logs, _ := manager.Logs(ctx, profile.Name, secrets)
		t.Fatalf("Dial = %v, want %s\n%s", err, FailureHandshakeTimeout, logs)
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
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })
	if err := manager.Start(ctx, profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}); err != nil {
		t.Fatalf("Start = %v", err)
	}

	// engineが起動し直された状況。前回のセッションのことは何も覚えていない。
	restarted := New(manager.directory, os.Getuid(), nil)
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
	requireHostDevice(t, backends[L2TPIPsec].device())
	profile := Profile{
		Name:    "l2tp-unreachable",
		Backend: L2TPIPsec,
		L2TP: &L2TPSettings{
			// TEST-NET-1。誰も応答しない。
			Server:   "192.0.2.1",
			Username: "fixture",
		},
	}
	secrets := Secrets{L2TP: &L2TPSecrets{Password: "fixture-password", PreSharedKey: "fixture-psk"}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	err := manager.Start(ctx, profile, secrets)

	requireFailureReason(t, err, FailureIPsecNegotiation)
	// 「成立しませんでした」は agent が標準エラーへ書く。docker logs の標準
	// 出力だけを読んでいると、この行が落ちる。
	logs := requireLogs(t, manager, ctx, profile.Name, secrets)
	if !strings.Contains(logs, "IPsecのネゴシエーションに失敗しました") {
		t.Fatalf("ログが IPsec の段階を指していない: %s", logs)
	}
	for _, forbidden := range []string{secrets.L2TP.Password, secrets.L2TP.PreSharedKey} {
		if strings.Contains(err.Error()+logs, forbidden) {
			t.Fatalf("見せる文面に秘密が現れた: %v / %s", err, logs)
		}
	}
}

// requireHostDevice は、トンネルのデバイスがこのマシンに無ければ飛ばす。Docker の
// テストは Linux でだけ走り、デバイスはホストのものが渡る。
func requireHostDevice(t *testing.T, device string) {
	t.Helper()
	if _, err := os.Stat(device); err != nil {
		t.Skipf("このマシンには %s が無い: %v", device, err)
	}
}

// requireFailureReason は、経路を用意できなかった理由が want であることを確かめる。
func requireFailureReason(t *testing.T, err error, want FailureReason) {
	t.Helper()
	var failure *SessionFailure
	if !errors.As(err, &failure) || failure.Reason != want {
		t.Fatalf("err = %v, want reason %s", err, want)
	}
}

// requireLogs は、終わったコンテナのログを、秘密を伏せて読む。
func requireLogs(t *testing.T, manager *Manager, ctx context.Context, profileName string, secrets Secrets) string {
	t.Helper()
	logs, err := manager.Logs(ctx, profileName, secrets)
	if err != nil {
		t.Fatalf("Logs = %v", err)
	}
	return logs
}

// openconnect の枝も、相手が応えなければそこで止まり、理由を残す。
//
// 実際のVPN装置に対しては確かめられていない。ここで確かめるのは、コンテナが
// openconnect を起動できること、応えない相手で待ち続けないこと、失敗の文面に
// パスワードが現れないことである。
func TestTheOpenConnectBranchRunsUntilTheServerRefusesIt(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[OpenConnect].device())
	profile := Profile{
		Name:    "oc-unreachable",
		Backend: OpenConnect,
		OpenConnect: &OpenConnectSettings{
			// TEST-NET-1。誰も応答しない。
			Server:   "192.0.2.1",
			Username: "fixture",
		},
	}
	secrets := Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	err := manager.Start(ctx, profile, secrets)

	requireFailureReason(t, err, FailureOpenConnect)
	// この行は agent が標準エラーへ書く。docker logs の標準出力だけを読んで
	// いると落ちる。
	logs := requireLogs(t, manager, ctx, profile.Name, secrets)
	if !strings.Contains(logs, "VPNサーバーへの接続に失敗しました") {
		t.Fatalf("ログが openconnect の段階を指していない: %s", logs)
	}
	if strings.Contains(err.Error()+logs, secrets.OpenConnect.Password) {
		t.Fatalf("見せる文面に秘密が現れた: %v / %s", err, logs)
	}
}

// 失敗の理由は、コンテナの標準エラーにある。
//
// docker logs は、コンテナの標準出力をこちらの標準出力へ、標準エラーをこちらの
// 標準エラーへ流す。片方だけを読むと、agent が書いた失敗の理由が落ちる。
func TestShownLogsCarryWhatTheContainerWroteToStandardError(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}

	shown, err := manager.docker.combined(ctx, "run", "--rm", "--entrypoint", "sh",
		image, "-c", "echo 出力; echo 理由 >&2")

	if err != nil {
		t.Fatalf("combined = %v", err)
	}
	for _, wanted := range []string{"出力", "理由"} {
		if !strings.Contains(shown, wanted) {
			t.Fatalf("combined = %q, %q が無い", shown, wanted)
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

	connection, err := manager.Dial(ctx, profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}},
		fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort))
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

// anyConnectServerAddress は、テスト用の AnyConnect 互換サーバーがトンネル側で
// 名乗るアドレスである。ocserv は ipv4-network の最初のアドレスを自分に使う。
const anyConnectServerAddress = "192.168.99.1"

// anyConnectTOTPSeed は、二段目のコードを作る種である（RFC 6238 の試験鍵）。
// users.oath へは同じ値を16進で書く。
const (
	anyConnectTOTPSeed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	anyConnectTOTPHex  = "3132333435363738393031323334353637383930"
)

// anyConnectServerName は、テスト用の AnyConnect 互換サーバーのコンテナ名である。
func anyConnectServerName() string {
	return fmt.Sprintf("sshc-vpn-test-anyconnect-%d", os.Getpid())
}

// startAnyConnectServer は、本物の AnyConnect 互換サーバー（ocserv）を1台立てる。
//
// パスワードのあとに OTP を聞く設定にする。openconnect がその二問目に答えられる
// ことが、この経路の要だからである。ocserv は製品のイメージには入れない。検査の
// ときだけ、相手側のコンテナへ入れる。
func startAnyConnectServer(
	t *testing.T, manager *Manager, ctx context.Context, image string,
) (bridgeAddress, certificatePin string) {
	t.Helper()
	name := anyConnectServerName()
	script := strings.Join([]string{
		"set -eu",
		"export DEBIAN_FRONTEND=noninteractive",
		// イメージは Ubuntu の snapshot から取る設定を持ち、CA 証明書を持たない。
		// 取り方は Dockerfile と同じにする（中身は apt が署名で確かめる）。
		"apt-get -o Acquire::https::Verify-Peer=false update -qq >/dev/null",
		"apt-get -o Acquire::https::Verify-Peer=false install -y -qq --no-install-recommends ocserv gnutls-bin >/dev/null",
		"mkdir -p /etc/ocserv",
		"cat > /tmp/ca.tmpl <<'EOF'",
		"cn = \"sshc test CA\"",
		"serial = 1",
		"expiration_days = 1",
		"ca",
		"signing_key",
		"cert_signing_key",
		"EOF",
		"cat > /tmp/server.tmpl <<'EOF'",
		"cn = \"sshc-test-vpn\"",
		"serial = 2",
		"expiration_days = 1",
		"signing_key",
		"encryption_key",
		"tls_www_server",
		"EOF",
		"certtool --generate-privkey --outfile /etc/ocserv/ca-key.pem",
		"certtool --generate-self-signed --load-privkey /etc/ocserv/ca-key.pem" +
			" --template /tmp/ca.tmpl --outfile /etc/ocserv/ca.pem",
		"certtool --generate-privkey --outfile /etc/ocserv/server-key.pem",
		"certtool --generate-certificate --load-privkey /etc/ocserv/server-key.pem" +
			" --load-ca-certificate /etc/ocserv/ca.pem --load-ca-privkey /etc/ocserv/ca-key.pem" +
			" --template /tmp/server.tmpl --outfile /etc/ocserv/server-cert.pem",
		"printf 'fixture-password\\nfixture-password\\n' | ocpasswd -c /etc/ocserv/passwd fixture",
		"printf 'HOTP/T30/6 fixture - " + anyConnectTOTPHex + "\\n' > /etc/ocserv/users.oath",
		"chmod 600 /etc/ocserv/users.oath",
		"cat > /etc/ocserv/ocserv.conf <<'EOF'",
		"auth = \"plain[passwd=/etc/ocserv/passwd,otp=/etc/ocserv/users.oath]\"",
		"tcp-port = 443",
		"udp-port = 443",
		"run-as-user = root",
		"run-as-group = root",
		"socket-file = /run/ocserv.socket",
		// 畳んだあとにセッションが残っていないかを occtl で確かめる。
		"use-occtl = true",
		"occtl-socket-file = /run/occtl.socket",
		"server-cert = /etc/ocserv/server-cert.pem",
		"server-key = /etc/ocserv/server-key.pem",
		"max-clients = 4",
		"max-same-clients = 4",
		"max-ban-score = 0",
		"device = vpns",
		"ipv4-network = 192.168.99.0",
		"ipv4-netmask = 255.255.255.0",
		"cisco-client-compat = true",
		"EOF",
		// トンネルの中からだけ届く相手。Dockerの通常回線からは届かない
		// アドレスなので、返事が来ればトンネルを通ったことになる。
		fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr SYSTEM:'echo tunnelled' &", echoPort),
		"exec ocserv --foreground --debug 1",
	}, "\n")
	if _, err := manager.docker.output(ctx, "run", "--detach", "--name", name,
		"--cap-add", "NET_ADMIN", "--device", "/dev/net/tun", "--network", "bridge",
		"--entrypoint", "sh", image, "-c", script); err != nil {
		t.Fatalf("AnyConnect互換サーバーを起動できない: %v", err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := manager.docker.combined(context.Background(), "logs", "--tail", "40", name)
			t.Logf("AnyConnect互換サーバーのログ:\n%s", logs)
		}
		_, _ = manager.docker.output(context.Background(), "rm", "--force", name)
	})
	address, err := manager.docker.output(ctx, "container", "inspect", "--format",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
	if err != nil {
		t.Fatal(err)
	}
	// 待つのは「証明書ができたこと」ではなく「待ち受けが始まったこと」である。
	// 証明書は ocserv を起こす前にできるので、そこで先へ進むと、まだ誰も
	// 聞いていない相手へ繋ぎに行く。apt の取得を含むので長めに待つ。
	deadline := time.Now().Add(4 * time.Minute)
	for {
		logs, err := manager.docker.combined(ctx, "logs", "--tail", "20", name)
		if err == nil && strings.Contains(logs, "listening (TCP)") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("AnyConnect互換サーバーが待ち受けを始めない: %v\n%s", err, logs)
		}
		time.Sleep(2 * time.Second)
	}
	pin, err := manager.docker.output(ctx, "exec", name, "sh", "-c",
		"certtool --certificate-info --infile /etc/ocserv/server-cert.pem"+
			" | grep -o 'pin-sha256:[A-Za-z0-9+/=]*' | head -1")
	if err != nil || !strings.HasPrefix(strings.TrimSpace(pin), "pin-sha256:") {
		t.Fatalf("AnyConnect互換サーバーの証明書の指紋が読めない: %q, %v", pin, err)
	}
	return strings.TrimSpace(address), strings.TrimSpace(pin)
}

// 本物の AnyConnect 互換サーバーへ、パスワードと二段目のコードで繋ぎ、その
// トンネルの中にいる相手へ engine が届く。
//
// 二段目の答えは標準入力の次の行として送る。承認を待つ設定（Duo の push など）
// も同じ道を通り、送る語が違うだけである。
func TestAConnectionReachesTheTargetThroughAnAnyConnectTunnel(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	image, err := manager.ensureImage(ctx)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	serverAddress, certificatePin := startAnyConnectServer(t, manager, ctx, image)

	profile := Profile{
		Name:    "anyconnect-e2e",
		Backend: OpenConnect,
		OpenConnect: &OpenConnectSettings{
			Server:            serverAddress,
			Username:          "fixture",
			ServerCertificate: certificatePin,
			SecondFactor:      SecondFactorTOTP,
		},
	}
	secrets := Secrets{OpenConnect: &OpenConnectSecrets{
		Password:   "fixture-password",
		TOTPSecret: anyConnectTOTPSeed,
	}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	requireAnswer(t, manager, ctx, dialTarget{profile: profile, secrets: secrets,
		address: fmt.Sprintf("%s:%d", anyConnectServerAddress, echoPort)}, "tunnelled")

	// 停止するときはVPNサーバーへ logout を伝える。伝えずに終わると、サーバーの側に
	// セッションが残り、同時接続の枠を使い続ける。
	if err := manager.Stop(ctx, profile.Name); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	users, err := manager.docker.output(ctx, "exec", anyConnectServerName(),
		"occtl", "--socket-file", "/run/occtl.socket", "--json", "show", "users")
	if err != nil {
		t.Fatalf("装置の利用者の一覧を読めない: %v", err)
	}
	if strings.Contains(users, "fixture") {
		t.Fatalf("畳んだあとも装置にセッションが残った: %s", users)
	}
}
