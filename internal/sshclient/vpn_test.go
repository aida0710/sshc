package sshclient_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
)

// VPN を指定した接続は、その名前の経路を通って接続先へ届く。
func TestAVPNTargetIsReachedThroughTheNamedRoute(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	dialer := dialerFor(t, server, auth)
	var asked []string
	dialer.DialVPN = func(ctx context.Context, profile, address string) (net.Conn, error) {
		asked = append(asked, profile+" "+address)
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	target := targetWith(server, path)
	target.VPN = "tohoku"

	connection, err := dialer.Connect(context.Background(), target)

	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	defer func() { _ = connection.Close() }()
	if len(asked) != 1 || asked[0] != "tohoku "+target.Address() {
		t.Fatalf("the VPN route was asked for %v", asked)
	}
}

// 経路を作れない engine は、VPN を指定した接続を黙って素の回線へ落とさない。
func TestAVPNTargetIsRefusedWhenTheEngineHasNoVPNTransport(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	dialer := dialerFor(t, server, auth)
	target := targetWith(server, path)
	target.VPN = "tohoku"

	connection, err := dialer.Connect(context.Background(), target)

	if connection != nil {
		_ = connection.Close()
		t.Fatal("a VPN target connected without a VPN transport")
	}
	if !errors.Is(err, sshclient.ErrVPNUnavailable) {
		t.Fatalf("Connect = %v, want ErrVPNUnavailable", err)
	}
}

// ProxyCommand はこの機械で走る。VPN の中には居ないので、両方は指定できない。
func TestAVPNTargetWithAProxyCommandIsRefused(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	dialer := dialerFor(t, server, auth)
	dialer.DialVPN = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("the VPN route was used for a target that also has a ProxyCommand")
		return nil, nil
	}
	target := targetWith(server, path)
	target.VPN = "tohoku"
	target.ProxyCommand = "/bin/false"

	connection, err := dialer.Connect(context.Background(), target)

	if connection != nil {
		_ = connection.Close()
	}
	if !errors.Is(err, sshclient.ErrVPNWithProxyCommand) {
		t.Fatalf("Connect = %v, want ErrVPNWithProxyCommand", err)
	}
}

// 踏み台の向こうのホップは、手前の SSH 接続の中を通る。この機械の VPN は効かない。
func TestAVPNOnAHopBeyondAJumpIsRefused(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	dialer := dialerFor(t, server, auth)
	dialer.DialVPN = func(ctx context.Context, profile, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	target := targetWith(server, path)
	target.VPN = "tohoku"
	target.Jump = []sshclient.Target{targetWith(server, path)}

	connection, err := dialer.Connect(context.Background(), target)

	if connection != nil {
		_ = connection.Close()
		t.Fatal("a hop beyond a jump was reached through a VPN profile")
	}
	if !errors.Is(err, sshclient.ErrVPNThroughJump) {
		t.Fatalf("Connect = %v, want ErrVPNThroughJump", err)
	}
}
