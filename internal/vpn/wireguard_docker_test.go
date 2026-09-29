//go:build linux

package vpn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// 設定ファイル（wg-quick の形）で作った WireGuard の経路を、本物のコンテナと本物の
// トンネルで確かめる。相手は session_docker_test.go と同じテスト用の WireGuard である。
// SSHC_VPN_DOCKER_TEST=1 のときだけ走る。

// wireGuardTestMTU は、設定ファイルに書くトンネルの MTU である。wireguard-go の既定（1420）
// と違う値にして、設定ファイルの MTU を使ったことを確かめる。
const wireGuardTestMTU = 1380

// newPresharedKey は、WireGuard の PresharedKey をひとつ作る。
func newPresharedKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// wireGuardConfigRoute は、設定ファイルの形のプロファイルと、そのシークレットである。
type wireGuardConfigRoute struct {
	profile Profile
	secrets Secrets
	// presharedKey は、設定ファイルに書いた PresharedKey である。
	presharedKey string
}

// newWireGuardConfigRoute は、PresharedKey を使うテスト用の相手を立て、それに繋ぐ設定ファイルを
// 作る。設定ファイルは、鍵を含む本文のままシークレットとして渡す。
func newWireGuardConfigRoute(t *testing.T, manager *Manager, ctx context.Context, name, allowedIPs string) wireGuardConfigRoute {
	t.Helper()
	image, err := manager.ensureImage(ctx, ignorePhases)
	if err != nil {
		t.Fatalf("イメージを用意できない: %v", err)
	}
	clientPrivate, clientPublic := keyPair(t)
	presharedKey := newPresharedKey(t)
	peerPublic, peerAddress := startTunnelPeerWithPresharedKey(t, manager, ctx, image, clientPublic, presharedKey)
	config := strings.Join([]string{
		"# プロバイダが配る形の設定ファイル",
		"[Interface]",
		"PrivateKey = " + clientPrivate,
		"Address = " + tunnelClientAddress + ", fd00::2/128",
		fmt.Sprintf("MTU = %d", wireGuardTestMTU),
		"Table = off",
		"",
		"[Peer]",
		"PublicKey = " + peerPublic,
		"PresharedKey = " + presharedKey,
		fmt.Sprintf("Endpoint = %s:%d", peerAddress, wireGuardPeerPort),
		"AllowedIPs = " + allowedIPs,
		"",
	}, "\n")
	t.Cleanup(func() { _ = manager.Stop(context.Background(), name) })
	return wireGuardConfigRoute{
		profile:      Profile{Name: name, Backend: WireGuard, WireGuard: &WireGuardSettings{Servers: []string{peerAddress}}},
		secrets:      Secrets{WireGuard: &WireGuardSecrets{Config: config}},
		presharedKey: presharedKey,
	}
}

// 設定ファイルの形で、PresharedKey を使う相手へ繋ぎ、トンネルの中の相手へ届く。MTU は設定
// ファイルの値になり、経路は接続先の /32 だけである。
func TestAWireGuardConfigWithAPresharedKeyCarriesAConnection(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	route := newWireGuardConfigRoute(t, manager, ctx, "wg-config", "10.77.0.0/24")

	requireAnswer(t, manager, ctx, dialTarget{profile: route.profile, secrets: route.secrets,
		address: fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort)}, "tunnelled")

	container := manager.containerName(route.profile.Name)
	link, err := manager.docker.output(ctx, "exec", container, "ip", "-o", "link", "show", "dev", "wg0")
	if err != nil || !strings.Contains(link, fmt.Sprintf("mtu %d", wireGuardTestMTU)) {
		t.Fatalf("wg0 の MTU が設定ファイルの値ではない: %v\n%s", err, link)
	}
	routes, err := manager.docker.output(ctx, "exec", container, "ip", "-4", "route", "show", "dev", "wg0")
	if err != nil || strings.TrimSpace(routes) != tunnelServerAddress+" scope link" {
		t.Fatalf("wg0 への経路が接続先の /32 だけではない: %v\n%s", err, routes)
	}
	// PresharedKey は相手に渡り、ログからは伏せてある。
	logs := requireLogs(t, manager, ctx, route.profile.Name, route.secrets)
	if strings.Contains(logs, route.presharedKey) {
		t.Fatalf("ログに PresharedKey が現れた:\n%s", logs)
	}
}

// AllowedIPs に無い接続先へは、WireGuard がパケットを送らない。繋ぎに行かずに、その理由を返す。
func TestATargetOutsideTheAllowedIPsSaysSo(t *testing.T) {
	manager, ctx := requireDockerTest(t)
	route := newWireGuardConfigRoute(t, manager, ctx, "wg-allowed", tunnelServerAddress+"/32")

	requireAnswer(t, manager, ctx, dialTarget{profile: route.profile, secrets: route.secrets,
		address: fmt.Sprintf("%s:%d", tunnelServerAddress, echoPort)}, "tunnelled")
	connection, err := manager.Dial(ctx, route.profile, route.secrets, "10.77.0.9:22")
	if err == nil {
		_ = connection.Close()
		t.Fatal("AllowedIPs に無い接続先へ繋いだ")
	}

	var failure *TargetFailure
	if !errors.As(err, &failure) || failure.Reason != FailureTargetNotAllowed {
		t.Fatalf("Dial = %v, want %s", err, FailureTargetNotAllowed)
	}
}
