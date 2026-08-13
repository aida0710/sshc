package sshclient_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// echoServer は、届いたバイト列に印を付けて返す本物の TCP 受け口である。
//
// これが転送の向こう側になる。**端から端まで見る**——設定に書いたポートへ
// 繋いだバイト列が、SSH のチャンネルを通ってここへ届くことを確かめる。
func echoServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buffer := make([]byte, 256)
				read, err := conn.Read(buffer)
				if err != nil {
					return
				}
				_, _ = conn.Write(append([]byte("echo:"), buffer[:read]...))
			}()
		}
	}()
	return listener.Addr().String()
}

// forwardingSession は、その転送を持つセッションを一本開く。
func forwardingSession(t *testing.T, forwards []sshclient.ForwardSpec) (terminal.Process, *testServer) {
	t.Helper()
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys:       []ssh.PublicKey{public},
		AllowDirectTCPIP: true,
		Reached:          map[string]func() net.Conn{},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.Forwards = forwards

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "ready")
	return process, server
}

// freePort は、いま誰も使っていないポートを返す。
func freePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	return port
}

func TestALocalForwardCarriesBytesToTheRemoteDestination(t *testing.T) {
	destination := echoServer(t)
	port := freePort(t)

	process, server := forwardingSession(t, []sshclient.ForwardSpec{{
		Kind: terminal.ForwardLocal, ListenPort: port, To: destination,
	}})
	server.allow(destination)

	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("the forwarded port is not open: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("through the tunnel")); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, 256)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	read, err := conn.Read(answer)
	if err != nil {
		t.Fatalf("nothing came back: %v", err)
	}
	if got := string(answer[:read]); got != "echo:through the tunnel" {
		t.Fatalf("the far side received %q", got)
	}
	// 転送は一覧に出る。**開いていることが見えないまま開かない。**
	forwards := process.(terminal.Forwarder).Forwards()
	if len(forwards) != 1 || forwards[0].Problem != "" || forwards[0].To != destination {
		t.Fatalf("forwards = %#v", forwards)
	}
}

// ポートが埋まっているのは普通の出来事である。**その転送だけが失敗し、
// 接続は続く。**
func TestABindFailureDoesNotEndTheSession(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	_, port, err := net.SplitHostPort(taken.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	process, _ := forwardingSession(t, []sshclient.ForwardSpec{{
		Kind: terminal.ForwardLocal, ListenPort: port, To: "10.0.0.5:80",
	}})

	forwards := process.(terminal.Forwarder).Forwards()
	if len(forwards) != 1 || forwards[0].Problem == "" {
		t.Fatalf("forwards = %#v, want the failure recorded", forwards)
	}
	// **セッションは生きている。** ポートひとつのためにコンソールを失わない。
	if _, err := process.Write([]byte("still here\r")); err != nil {
		t.Fatalf("the session died with the forward: %v", err)
	}
}

// **閉じ忘れたポートは、そのプロセスが死ぬまで埋まる。**
func TestClosingTheSessionReleasesTheForwardedPort(t *testing.T) {
	port := freePort(t)
	process, _ := forwardingSession(t, []sshclient.ForwardSpec{{
		Kind: terminal.ForwardLocal, ListenPort: port, To: "10.0.0.5:80",
	}})

	if conn, err := net.Dial("tcp", "127.0.0.1:"+port); err != nil {
		t.Fatalf("the forwarded port is not open: %v", err)
	} else {
		_ = conn.Close()
	}

	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err == nil {
			_ = listener.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the port is still held after the session closed")
}

func TestADynamicForwardSpeaksSOCKS5Connect(t *testing.T) {
	destination := echoServer(t)
	port := freePort(t)

	_, server := forwardingSession(t, []sshclient.ForwardSpec{{
		Kind: terminal.ForwardDynamic, ListenPort: port,
	}})
	server.allow(destination)

	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatalf("the SOCKS port is not open: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatal(err)
	}
	if greeting[0] != 0x05 || greeting[1] != 0x00 {
		t.Fatalf("greeting = %v", greeting)
	}

	host, portText, err := net.SplitHostPort(destination)
	if err != nil {
		t.Fatal(err)
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	request = append(request, host...)
	number := make([]byte, 2)
	value, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint16(number, uint16(value))
	request = append(request, number...)
	if _, err := conn.Write(request); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("SOCKS refused with %d", reply[1])
	}

	if _, err := conn.Write([]byte("socks payload")); err != nil {
		t.Fatal(err)
	}
	answer := make([]byte, 256)
	read, err := conn.Read(answer)
	if err != nil {
		t.Fatalf("nothing came back: %v", err)
	}
	if got := string(answer[:read]); got != "echo:socks payload" {
		t.Fatalf("the far side received %q", got)
	}
}

// **CONNECT だけを受ける。** 持たない機能を受け付けて黙って失敗するより断る。
func TestADynamicForwardRefusesCommandsItDoesNotHave(t *testing.T) {
	port := freePort(t)
	forwardingSession(t, []sshclient.ForwardSpec{{Kind: terminal.ForwardDynamic, ListenPort: port}})

	conn, err := net.Dial("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	// BIND を頼む。
	if _, err := conn.Write([]byte{0x05, 0x02, 0x00, 0x01, 127, 0, 0, 1, 0x00, 0x50}); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] == 0x00 {
		t.Fatal("BIND was accepted")
	}
}

// 転送先へ繋がらないのは、その 1 本の問題である。listener は生きている。
func TestAnUnreachableDestinationLeavesTheListenerOpen(t *testing.T) {
	port := freePort(t)
	forwardingSession(t, []sshclient.ForwardSpec{{
		Kind: terminal.ForwardLocal, ListenPort: port, To: "10.0.0.5:80",
	}})

	for attempt := 0; attempt < 2; attempt++ {
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatalf("attempt %d: the listener died: %v", attempt, err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Read(make([]byte, 1)); err == nil {
			t.Errorf("attempt %d: a connection to nowhere returned data", attempt)
		}
		_ = conn.Close()
	}
}

func TestForwardSpecificationsAreReadTheWayOpenSSHWritesThem(t *testing.T) {
	for _, test := range []struct {
		value  string
		listen string
		to     string
		folded bool
	}{
		{value: "8080 10.0.0.5:80", listen: "8080", to: "10.0.0.5:80"},
		{value: "127.0.0.1:8080 10.0.0.5:80", listen: "8080", to: "10.0.0.5:80"},
		{value: "localhost:8080 10.0.0.5:80", listen: "8080", to: "10.0.0.5:80"},
		{value: "0.0.0.0:8080 10.0.0.5:80", listen: "8080", to: "10.0.0.5:80", folded: true},
		{value: "[::1]:8080 [fd00::1]:80", listen: "8080", to: "[fd00::1]:80"},
	} {
		spec, err := sshclient.ParseLocalForward(test.value)
		if err != nil {
			t.Errorf("ParseLocalForward(%q) = %v", test.value, err)
			continue
		}
		if spec.ListenPort != test.listen || spec.To != test.to {
			t.Errorf("ParseLocalForward(%q) = %#v", test.value, spec)
		}
		if spec.Bound() != test.folded {
			t.Errorf("ParseLocalForward(%q).Bound() = %v, want %v", test.value, spec.Bound(), test.folded)
		}
	}

	for _, value := range []string{"", "8080", "nonsense here", "0 10.0.0.5:80", "8080 10.0.0.5"} {
		if _, err := sshclient.ParseLocalForward(value); err == nil {
			t.Errorf("ParseLocalForward(%q) was accepted", value)
		}
	}
	if spec, err := sshclient.ParseDynamicForward("1080"); err != nil || spec.ListenPort != "1080" {
		t.Errorf("ParseDynamicForward = %#v, %v", spec, err)
	}
	if _, err := sshclient.ParseDynamicForward("1080 extra"); err == nil {
		t.Error("ParseDynamicForward accepted a destination it cannot use")
	}
	if !strings.Contains(sshclient.LoopbackHost, "127.") {
		t.Error("forwards must bind to loopback")
	}
}

// **鍵そのものは渡らない。渡るのは鍵を使う権利である。** リモートが
// auth-agent@openssh.com を開くと、その要求はこのチャンネルを通ってこちらの
// agent へ届く。
func TestAgentForwardingLendsTheAgentToTheRemote(t *testing.T) {
	socket, agentKey := runTestAgent(t, "")
	path, contents, public := keyPair(t)

	seen := make(chan string, 1)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
		OnAgentChannel: func(conn net.Conn) {
			// リモート側から、借りた agent に鍵を尋ねる。
			loaded, err := agent.NewClient(conn).List()
			if err != nil || len(loaded) == 0 {
				seen <- ""
				return
			}
			seen <- ssh.FingerprintSHA256(loaded[0])
		},
	})
	auth := sshclient.Auth{
		ReadFile:    func(string) ([]byte, error) { return contents, nil },
		AgentSocket: socket,
	}
	target := targetWith(server, path)
	target.AgentForward = true

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	readUntil(t, process, "ready")

	select {
	case fingerprint := <-seen:
		if fingerprint != ssh.FingerprintSHA256(agentKey.PublicKey()) {
			t.Fatalf("the remote saw %q", fingerprint)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the remote never reached the forwarded agent")
	}

	forwards := process.(terminal.Forwarder).Forwards()
	if len(forwards) != 1 || forwards[0].Kind != terminal.ForwardAgent || forwards[0].Problem != "" {
		t.Fatalf("forwards = %#v", forwards)
	}
}

// **転送できないことを理由に接続を断らない。**
func TestAgentForwardingWithoutAnAgentStillConnects(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.AgentForward = true

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	seen := readUntil(t, process, "ready")
	if !strings.Contains(seen, "no agent is reachable") {
		t.Errorf("the terminal does not say why the agent was not forwarded: %q", seen)
	}
	forwards := process.(terminal.Forwarder).Forwards()
	if len(forwards) != 1 || forwards[0].Problem == "" {
		t.Fatalf("forwards = %#v, want the reason recorded", forwards)
	}
}
