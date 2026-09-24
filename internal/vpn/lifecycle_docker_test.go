//go:build linux

package vpn

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// 経路の寿命を、本物のコンテナで確かめる。SSHC_VPN_DOCKER_TEST=1 のときだけ走る。

// wireGuardRoute は、テスト用の相手へ繋ぐ wireguard の経路と、その秘密を返す。
func wireGuardRoute(t *testing.T, manager *Manager, ctx context.Context, name string) (Profile, Secrets) {
	t.Helper()
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	peerPublic, peerAddress := startTunnelPeer(t, manager, ctx, image, clientPublic)
	profile := Profile{
		Name:    name,
		Backend: WireGuard,
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: peerAddress, Port: 51820},
			PeerPublicKey: peerPublic,
			Address:       tunnelClientAddress,
		},
	}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })
	return profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: clientPrivate}}
}

// CLI とホストの ssh は engine の中継へ繋ぐ。その接続も数え、通っているあいだは畳まない。
func TestAConnectionThroughTheEngineRelayKeepsTheRouteUp(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	profile, secrets := wireGuardRoute(t, manager, ctx, "relayed")
	clock := &manualClock{at: time.Now()}
	manager.now = clock.now

	// `vpn up` と同じく、起こすだけで接続はしない。
	if err := manager.Start(ctx, profile, secrets); err != nil {
		t.Fatalf("Start = %v", err)
	}
	status, err := manager.Status(ctx, profile.Name)
	if err != nil || status.RelaySocket == "" {
		t.Fatalf("Status = %+v, %v", status, err)
	}
	if !strings.HasSuffix(status.RelaySocket, engineRelaySocketName) {
		t.Fatalf("見せている中継が engine のものではない: %q", status.RelaySocket)
	}
	connection, err := net.Dial("unix", status.RelaySocket)
	if err != nil {
		t.Fatalf("engine の中継へ繋げない: %v", err)
	}
	_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	if err := WriteRelayRequest(connection, fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort)); err != nil {
		t.Fatal(err)
	}
	if reply, err := ReadRelayReply(connection); err != nil || reply.Code != "" {
		t.Fatalf("engine の中継の答え = %+v, %v", reply, err)
	}
	answer := make([]byte, 9)
	if _, err := io.ReadFull(connection, answer); err != nil || string(answer) != "tunnelled" {
		t.Fatalf("engine の中継越しの返事 = %q, %v", answer, err)
	}

	clock.advance(time.Hour)
	manager.StopIdle(ctx, time.Minute)
	if status, err := manager.Status(ctx, profile.Name); err != nil || !status.Running {
		t.Fatalf("engine の中継を通る接続があるのに畳んだ: %+v, %v", status, err)
	}

	_ = connection.Close()
	// engine の中継は、相手が閉じたことを受けてから借りを返す。
	waitUntil(t, func() bool { return openConnections(manager.state(profile.Name)) == 0 })
	clock.advance(time.Hour)
	manager.StopIdle(ctx, time.Minute)
	if status, err := manager.Status(ctx, profile.Name); err != nil || status.Running || status.RelaySocket != "" {
		t.Fatalf("誰も通っていない経路が残った: %+v, %v", status, err)
	}
}

// 起動を途中でやめても、コンテナは残らない。
func TestACancelledStartLeavesNoContainer(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	requireHostDevice(t, backends[L2TPIPsec].device())
	if _, err := manager.ensureImage(ctx, ignorePhases); err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	profile := Profile{
		Name:    "cancelled",
		Backend: L2TPIPsec,
		// TEST-NET-1。誰も応答しないので、IPsec を待ち続ける。
		L2TP: &L2TPSettings{Server: "192.0.2.1", Username: "fixture"},
	}
	secrets := Secrets{L2TP: &L2TPSecrets{Password: "fixture-password", PreSharedKey: "fixture-psk"}}
	t.Cleanup(func() { _ = manager.Stop(context.Background(), profile.Name) })

	starting, cancel := context.WithCancel(ctx)
	go func() {
		// トンネルを待つ段まで進んでから取り消す。
		waitUntil(t, func() bool { return manager.state(profile.Name).currentPhase() == PhaseTunnel })
		cancel()
	}()
	if err := manager.Start(starting, profile, secrets); err == nil {
		t.Fatal("取り消した起動が成功した")
	}

	// 止まったまま残ったコンテナも、残ったうちに入る。
	ours, err := manager.requireOurContainer(ctx, manager.containerName(profile.Name), profile.Name)
	if err != nil || ours {
		t.Fatalf("取り消した起動のコンテナが残った: %v, %v", ours, err)
	}
}

// 同じ利用者でも、別の workspace の engine が立てたコンテナには触れない。
func TestAnotherWorkspacesRouteIsNotDiscarded(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	profile, secrets := wireGuardRoute(t, manager, ctx, "neighbour")
	if err := manager.Start(ctx, profile, secrets); err != nil {
		t.Fatalf("Start = %v", err)
	}

	other := New(t.TempDir(), os.Getuid(), nil)
	if err := other.DiscardOrphans(ctx); err != nil {
		t.Fatalf("DiscardOrphans = %v", err)
	}

	if status, err := manager.Status(ctx, profile.Name); err != nil || !status.Running {
		t.Fatalf("別の workspace の回収で止められた: %+v, %v", status, err)
	}
}

// manualClock は、手で進める時計である。engine の中継の goroutine も読むので鍵で守る。
type manualClock struct {
	mutex sync.Mutex
	at    time.Time
}

func (clock *manualClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.at
}

func (clock *manualClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.at = clock.at.Add(duration)
}

// openConnections は、経路を通っている接続と予約の数を読む。
func openConnections(state *sessionState) int {
	state.use.Lock()
	defer state.use.Unlock()
	return state.open
}

// waitUntil は、条件が成り立つまで少しずつ待つ。
func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(dockerTestTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Error("条件が成り立たないまま時間切れになった")
			return
		}
		time.Sleep(readyPollInterval)
	}
}

// stopAllowance は、経路を畳むのにかかってよい長さである。agent が合図を受けて
// すぐ終わるなら、docker stop の猶予（10秒）を待ち切ることはない。
const stopAllowance = 5 * time.Second

// 経路は合図を受けてすぐ畳まれる。docker stop の猶予を待ち切らない。
func TestARouteStopsWithoutWaitingOutTheStopTimeout(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	profile, secrets := wireGuardRoute(t, manager, ctx, "quick-stop")
	if err := manager.Start(ctx, profile, secrets); err != nil {
		t.Fatalf("Start = %v", err)
	}

	started := time.Now()
	if err := manager.Stop(ctx, profile.Name); err != nil {
		t.Fatalf("Stop = %v", err)
	}

	if took := time.Since(started); took > stopAllowance {
		t.Fatalf("畳むのに %v かかった（上限 %v）", took, stopAllowance)
	}
}

// 前の版のイメージは、新しいイメージを作ったあとに消す。
func TestImagesOfEarlierVersionsAreRemoved(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	current, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	stale := imageName + ":stale-test"
	if _, err := manager.docker.output(ctx, "image", "tag", current, stale); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = manager.docker.output(context.Background(), "image", "rm", stale) })

	manager.removeOtherImages(ctx, current)

	if _, err := manager.docker.output(ctx, "image", "inspect", stale); err == nil {
		t.Fatal("前の版のイメージが残った")
	}
	if _, err := manager.docker.output(ctx, "image", "inspect", current); err != nil {
		t.Fatalf("いまのイメージまで消した: %v", err)
	}
}

// 一覧は、この engine の経路の状態をまとめて返す。
func TestStatusesListTheRoutesOfThisEngine(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	profile, secrets := wireGuardRoute(t, manager, ctx, "listed")
	if err := manager.Start(ctx, profile, secrets); err != nil {
		t.Fatalf("Start = %v", err)
	}

	statuses, err := manager.Statuses(ctx)
	if err != nil {
		t.Fatalf("Statuses = %v", err)
	}
	status := statuses[profile.Name]
	if !status.Running || status.RelaySocket == "" {
		t.Fatalf("status = %+v", status)
	}
	if status.Tunnel.Interface != "wg0" {
		t.Fatalf("tunnel = %+v", status.Tunnel)
	}
}
