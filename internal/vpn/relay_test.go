package vpn

import (
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// engine の中継は、受けた接続を経路の接続とつなぎ、両方向へ運ぶ。
func TestTheEngineRelayCarriesBytesBothWays(t *testing.T) {
	directory := shortSocketDirectory(t)
	route, routeSide := net.Pipe()
	defer func() { _ = routeSide.Close() }()
	relay, err := openEngineRelay(filepath.Join(directory, engineRelaySocketName),
		func() (net.Conn, error) { return route, nil })
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	defer func() { _ = relay.close() }()

	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	go func() {
		received := make([]byte, 5)
		if _, err := io.ReadFull(routeSide, received); err == nil {
			_, _ = routeSide.Write([]byte(strings.ToUpper(string(received))))
		}
	}()

	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, 5)
	if _, err := io.ReadFull(client, answer); err != nil || string(answer) != "HELLO" {
		t.Fatalf("answer = %q, %v", answer, err)
	}
}

// 経路へ繋げなければ、受けた接続はすぐ閉じる。待たせたままにしない。
func TestTheEngineRelayClosesAClientWhenTheRouteCannotBeReached(t *testing.T) {
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
		func() (net.Conn, error) { return nil, errors.New("no route") })
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	defer func() { _ = relay.close() }()

	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("Read = %v, want EOF", err)
	}
}

// ソケットのパスが長すぎる場所では、経路を起こす前に断る。
func TestADeepWorkspaceIsRefusedBeforeTheRouteStarts(t *testing.T) {
	deep := "/" + strings.Repeat("d", maxSocketPathLength)

	if err := requireSocketPaths(deep); !errors.Is(err, ErrSocketPath) {
		t.Fatalf("requireSocketPaths = %v", err)
	}
	if err := requireSocketPaths("/home/user/.ssh/sshc/vpn/lab"); err != nil {
		t.Fatalf("requireSocketPaths = %v", err)
	}
}
