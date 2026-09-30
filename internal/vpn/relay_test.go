package vpn

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// dialEngineRelay は、engine の中継へ繋ぎ、1行目に address を送って答えを読む。
func dialEngineRelay(t *testing.T, relay *engineRelay, address string) (net.Conn, RelayReply) {
	t.Helper()
	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if err := WriteRelayRequest(client, address); err != nil {
		t.Fatal(err)
	}
	reply, err := ReadRelayReply(client)
	if err != nil {
		t.Fatalf("ReadRelayReply = %v", err)
	}
	return client, reply
}

// engine の中継は、1行目の接続先へ繋ぎ、答えを返してから両方向へ運ぶ。
func TestTheEngineRelayCarriesBytesToTheRequestedDestination(t *testing.T) {
	route, routeSide := net.Pipe()
	defer func() { _ = routeSide.Close() }()
	requested := make(chan string, 1)
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
		func(_ context.Context, address string) (net.Conn, error) {
			requested <- address
			return route, nil
		}, relayRequestTimeout)
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	defer func() { _ = relay.close() }()

	client, reply := dialEngineRelay(t, relay, "10.9.9.1:22")
	if reply.Code != "" {
		t.Fatalf("reply = %+v", reply)
	}
	if address := <-requested; address != "10.9.9.1:22" {
		t.Fatalf("dialed %q", address)
	}
	go func() {
		received := make([]byte, 5)
		if _, err := io.ReadFull(routeSide, received); err == nil {
			_, _ = routeSide.Write([]byte(strings.ToUpper(string(received))))
		}
	}()
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, 5)
	if _, err := io.ReadFull(client, answer); err != nil || string(answer) != "HELLO" {
		t.Fatalf("answer = %q, %v", answer, err)
	}
}

// 接続先へ繋げなければ、理由の語を答えてから閉じる。待たせたままにしない。
func TestTheEngineRelayAnswersWhyItCouldNotReachTheDestination(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		code   string
		reason string
	}{
		{"名前解決の失敗", &TargetFailure{Profile: "lab", Destination: "db:22", Reason: FailureTargetUnresolved},
			CodeTargetFailed, string(FailureTargetUnresolved)},
		{"DNSの無い名前", &DestinationError{Address: "db:22", Reason: ReasonNameNeedsDNS},
			CodeDestinationInvalid, string(ReasonNameNeedsDNS)},
		{"トンネルの切断", &RouteFailure{Profile: "lab", Reason: FailureTunnelLost},
			CodeRouteFailed, string(FailureTunnelLost)},
		{"分からない失敗", errors.New("docker is gone"), CodeRouteFailed, string(FailureUnknown)},
	} {
		t.Run(test.name, func(t *testing.T) {
			relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
				func(context.Context, string) (net.Conn, error) { return nil, test.err }, relayRequestTimeout)
			if err != nil {
				t.Fatalf("openEngineRelay = %v", err)
			}
			defer func() { _ = relay.close() }()

			client, reply := dialEngineRelay(t, relay, "db:22")
			if reply.Code != test.code || reply.Reason != test.reason {
				t.Fatalf("reply = %+v", reply)
			}
			if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("Read = %v, want EOF", err)
			}
		})
	}
}

// 1行目が長すぎれば、繋ぎに行かずに閉じる。
func TestTheEngineRelayRefusesAnOverlongRequest(t *testing.T) {
	var dialed atomic.Bool
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
		func(context.Context, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unreachable")
		}, relayRequestTimeout)
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = client.Write([]byte(strings.Repeat("a", maxRelayLineBytes+10) + "\n"))
	// 読み残しがあるまま閉じるので、EOF ではなくリセットで届くことがある。
	// どちらでも、答えの行は来ない。
	if read, err := client.Read(make([]byte, 1)); read != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read = %d, %v, want the connection closed", read, err)
	}
	// 受けた接続は、繋ぎに行く前に閉じられている。
	_ = relay.close()
	if dialed.Load() {
		t.Fatal("dialed for an overlong request")
	}
}

// halfClosingRoute は、engine の中継が数える接続の形（countedConnection）で返す経路と、
// その向こう側（コンテナの中継の代わり）である。
func halfClosingRoute(t *testing.T) (relayDialer, <-chan *net.UnixConn) {
	t.Helper()
	listener, err := net.Listen("unix", filepath.Join(shortSocketDirectory(t), "route.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	far := make(chan *net.UnixConn, 1)
	go func() {
		if accepted, err := listener.Accept(); err == nil {
			far <- accepted.(*net.UnixConn)
		}
	}()
	dial := func(ctx context.Context, _ string) (net.Conn, error) {
		near, err := (&net.Dialer{}).DialContext(ctx, "unix", listener.Addr().String())
		if err != nil {
			return nil, err
		}
		return &countedConnection{Conn: near, release: func() {}}, nil
	}
	return dial, far
}

// 送る側の終わりは、両方向とも相手へ届く。届かないと、入力の終わりを待って答える
// 相手とのあいだで、sshc vpn proxy と中継が止まったままになる。
func TestTheEngineRelayPassesTheEndOfInputBothWays(t *testing.T) {
	dial, far := halfClosingRoute(t)
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName), dial, relayRequestTimeout)
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	defer func() { _ = relay.close() }()
	client, reply := dialEngineRelay(t, relay, "10.9.9.1:22")
	if reply.Code != "" {
		t.Fatalf("reply = %+v", reply)
	}
	var route *net.UnixConn
	select {
	case route = <-far:
	case <-time.After(promptly):
		t.Fatal("経路へ繋がらない")
	}
	defer func() { _ = route.Close() }()
	_ = route.SetDeadline(time.Now().Add(promptly))

	if err := client.(*net.UnixConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if read, err := route.Read(make([]byte, 1)); read != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("経路の側の Read = %d, %v, want EOF", read, err)
	}
	if _, err := route.Write([]byte("bye")); err != nil {
		t.Fatal(err)
	}
	if err := route.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	received, err := io.ReadAll(client)
	if err != nil || string(received) != "bye" {
		t.Fatalf("クライアントの側が受け取ったもの = %q, %v", received, err)
	}
}

// 繋いだまま1行目を送らない接続は、上限を過ぎたら閉じ、接続先へは繋ぎに行かない。
func TestTheEngineRelayClosesAConnectionThatNeverSendsTheFirstLine(t *testing.T) {
	var dialed atomic.Bool
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
		func(context.Context, string) (net.Conn, error) {
			dialed.Store(true)
			return nil, errors.New("unreachable")
		}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	defer func() { _ = relay.close() }()
	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_ = client.SetDeadline(time.Now().Add(promptly))

	if read, err := client.Read(make([]byte, 1)); read != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read = %d, %v, want the connection closed", read, err)
	}
	if dialed.Load() {
		t.Fatal("1行目の無い接続のために接続先へ繋ぎに行った")
	}
}

// 中継を閉じると、接続先へ繋ぎに行っている途中の接続も打ち切り、繋いだ側を待たせない。
func TestClosingTheEngineRelayCancelsADialInProgress(t *testing.T) {
	entered := make(chan struct{})
	dialEnded := make(chan error, 1)
	relay, err := openEngineRelay(filepath.Join(shortSocketDirectory(t), engineRelaySocketName),
		func(ctx context.Context, _ string) (net.Conn, error) {
			close(entered)
			<-ctx.Done()
			dialEnded <- ctx.Err()
			return nil, ctx.Err()
		}, relayRequestTimeout)
	if err != nil {
		t.Fatalf("openEngineRelay = %v", err)
	}
	client, err := net.Dial("unix", relay.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	_ = client.SetDeadline(time.Now().Add(promptly))
	if err := WriteRelayRequest(client, "10.9.9.1:22"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(promptly):
		t.Fatal("接続先へ繋ぎに行かない")
	}

	if err := relay.close(); err != nil {
		t.Fatalf("close = %v", err)
	}

	if err := receiveWithin(t, dialEnded, "dial"); !errors.Is(err, context.Canceled) {
		t.Fatalf("dial が終わった理由 = %v, want context.Canceled", err)
	}
	reply, err := ReadRelayReply(client)
	if err != nil || reply.Code != CodeRouteFailed {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	if read, err := client.Read(make([]byte, 1)); read != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read = %d, %v, want the connection closed", read, err)
	}
}

// ソケットのパスが長すぎる場所では、経路を起こす前に断る。
func TestADeepWorkspaceIsRefusedBeforeTheRouteStarts(t *testing.T) {
	deep := "/" + strings.Repeat("d", maxSocketPathLength)

	if err := requireSocketPath(deep); !errors.Is(err, ErrSocketPath) {
		t.Fatalf("requireSocketPath = %v", err)
	}
	if err := requireSocketPath("/home/user/.ssh/sshc/vpn/lab"); err != nil {
		t.Fatalf("requireSocketPath = %v", err)
	}
}
