package sshclient_test

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

type closeObservedConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *closeObservedConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// dialerFor は、このサーバーのホスト鍵を既知として組み立てた Dialer である。
func dialerFor(t *testing.T, server *testServer, auth sshclient.Auth) sshclient.Dialer {
	t.Helper()
	known := knownHostsLine(server.Host(), server.HostKey.PublicKey())
	if server.Port() != "22" {
		known = knownHostsLine("["+server.Host()+"]:"+server.Port(), server.HostKey.PublicKey())
	}
	return sshclient.Dialer{
		Auth:     auth,
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
	}
}

// keyPair は、この検査が使う鍵ひとつをメモリ上に作る。
func keyPair(t *testing.T) (path string, contents []byte, public ssh.PublicKey) {
	t.Helper()
	private, err := keys.GeneratePrivateKey(keys.AlgorithmEd25519, 0, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := keys.EncodePrivateKey(private, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	return "/keys/id_ed25519", encoded, signer.PublicKey()
}

// drain は、この Process を読み続ける goroutine を置く。
//
// 本番では terminal.Registry の pump がこれをしている。出力の道は
// io.Pipe なので、誰も読まない Process は書き込みで止まる。読み手を置かない
// 検査は、その事実を「動かない」と誤って報告する。
func drain(process terminal.Process) {
	go func() { _, _ = io.Copy(io.Discard, process) }()
}

// awaitReady は、SSH の Process が使える状態になるか失敗するまで待ち、その結果を返す。
func awaitReady(process terminal.Process) error {
	readier := process.(terminal.Readier)
	<-readier.Ready()
	return readier.ReadyErr()
}

// readUntil は、その断片が現れるまで Process を読む。
func readUntil(t *testing.T, process terminal.Process, wanted string) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var seen strings.Builder
	buffer := make([]byte, 4096)
	for time.Now().Before(deadline) {
		read, err := process.Read(buffer)
		seen.Write(buffer[:read])
		if strings.Contains(seen.String(), wanted) {
			return seen.String()
		}
		if err != nil {
			t.Fatalf("read stopped before %q appeared: %v (saw %q)", wanted, err, seen.String())
		}
	}
	t.Fatalf("%q never appeared (saw %q)", wanted, seen.String())
	return ""
}

func TestOpenRunsAShellAndCarriesItsOutput(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "welcome to the fixture\r\n")
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)

	process, err := dialerFor(t, server, auth).Open(context.Background(), target, terminal.Size{Cols: 120, Rows: 40})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "welcome to the fixture")
	if info := process.Wait(); info.Code != 0 {
		t.Errorf("exit = %+v, want a clean exit", info)
	}
	// 端末の大きさは pty-req で届く。届かないと vim も top も壊れた幅で描く。
	term, size := server.PTY()
	if term != terminal.DefaultTerminalType || size != [2]uint32{120, 40} {
		t.Errorf("pty-req = %q %v", term, size)
	}
	modes := server.PTYModes()
	if len(modes) != 1 || modes[ssh.ECHO] != 1 {
		t.Errorf("pty modes = %v, want only ECHO=1", modes)
	}
	if _, exists := modes[ssh.TTY_OP_ISPEED]; exists {
		t.Error("pty request invented an input baud rate")
	}
	if _, exists := modes[ssh.TTY_OP_OSPEED]; exists {
		t.Error("pty request invented an output baud rate")
	}
}

func TestTheRemoteExitCodeReachesTheSessionListing(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}, ExitCode: 42})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	if info := process.Wait(); info.Code != 42 {
		t.Fatalf("exit = %+v, want 42", info)
	}
}

func TestAShellEndingWithoutExitStatusOnALiveTransportIsNotALostConnection(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public}, OmitExitStatus: true,
		OnShell: func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	output := readUntil(t, process, "ready")
	rest, _ := io.ReadAll(process)
	info := process.Wait()
	if info.TransportLost {
		t.Fatalf("exit = %+v; a shell that ended on a live transport was reported as a lost connection", info)
	}
	if info.Code != sshclient.RemoteFailureExit {
		t.Fatalf("exit = %+v, want %d", info, sshclient.RemoteFailureExit)
	}
	// 終わり方の文は出力に書かずに返す。engine がモードを戻してから書く。
	if output += string(rest); strings.Contains(output, "終了コードを送らずに") {
		t.Fatalf("output = %q; the notice was written before the modes could be reset", output)
	}
	if !strings.Contains(info.Notice, "終了コードを送らずに") {
		t.Fatalf("notice = %q, want the server closed the session without an exit status", info.Notice)
	}
}

func TestATransportCutDuringTheShellIsALostConnection(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	dialed := make(chan net.Conn, 1)
	dialer := dialerFor(t, server, auth)
	dialer.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			dialed <- conn
		}
		return conn, err
	}

	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	output := readUntil(t, process, "ready")
	_ = (<-dialed).Close()
	rest, _ := io.ReadAll(process)
	info := process.Wait()
	if !info.TransportLost {
		t.Fatalf("exit = %+v; a cut transport was reported as the end of the shell", info)
	}
	// 切れた理由は出力に書かずに返す。engine がモードを戻してから書く。
	if strings.TrimSpace(info.Notice) == "" {
		t.Fatalf("exit = %+v; the reason the transport was lost is missing", info)
	}
	if output += string(rest); strings.Contains(output, strings.TrimSpace(info.Notice)) {
		t.Fatalf("output = %q; the reason was written before the modes could be reset", output)
	}
}

func TestWhatIsTypedReachesTheRemoteStdin(t *testing.T) {
	path, contents, public := keyPair(t)
	echoed := make(chan string, 1)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			line := make([]byte, 64)
			read, _ := channel.Read(line)
			echoed <- string(line[:read])
			_, _ = channel.Write(line[:read])
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	// シェルが始まってから書く。Ready の前に書いたぶんは、認証の問いが出て
	// いなければ捨てられる。
	if err := awaitReady(process); err != nil {
		t.Fatalf("the shell never became ready: %v", err)
	}
	if _, err := process.Write([]byte("echo hello\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-echoed:
		if got != "echo hello\r" {
			t.Fatalf("the remote received %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("nothing reached the remote stdin")
	}
}

func TestOnlyAnExplicitAuthenticationPromptAcceptsInputBeforeReady(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	process, err := dialerFor(t, server, sshclient.Auth{}).Open(
		context.Background(), targetWith(server), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	if _, ok := process.(terminal.Prompting); !ok {
		t.Fatal("SSH process does not expose its prompt state")
	}
	readUntil(t, process, "Password for ")
	prompting := process.(terminal.Prompting)
	if !prompting.AwaitingPrompt() {
		t.Fatal("password prompt was not marked as awaiting input")
	}
	drain(process)
	if _, err := process.Write([]byte("hunter2\r")); err != nil {
		t.Fatal(err)
	}
	if readyErr := awaitReady(process); readyErr != nil {
		t.Fatalf("Ready = %v", readyErr)
	}
	if prompting.AwaitingPrompt() {
		t.Fatal("prompt remained active after authentication")
	}
}

func TestInputTypedAfterAPasswordAnswerReachesTheNewShell(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Password: "hunter2",
		OnShell: func(channel ssh.Channel) {
			line := make([]byte, 64)
			read, _ := channel.Read(line)
			_, _ = channel.Write(line[:read])
		},
	})
	process, err := dialerFor(t, server, sshclient.Auth{}).Open(
		context.Background(), targetWith(server), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "Password for ")
	if _, err := process.Write([]byte("hunter2\recho after-auth\r")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, process, "echo after-auth")
}

func TestResizeSendsAWindowChange(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			// 開いたままにする。閉じたセッションへ window-change は送れない。
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "ready")
	if err := process.Resize(terminal.Size{Cols: 200, Rows: 60}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, size := range server.Sizes() {
			if size == [2]uint32{200, 60} {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("window-change never arrived: %v", server.Sizes())
}

func TestResizeDuringSetEnvReachesTheRemotePTY(t *testing.T) {
	path, contents, public := keyPair(t)
	envArrived := make(chan struct{})
	releaseEnv := make(chan struct{})
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		BeforeEnvReply: func() {
			close(envArrived)
			<-releaseEnv
		},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.SetEnv = []sshclient.EnvVar{{Name: "SSHC", Value: "yes"}}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	select {
	case <-envArrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the env request never arrived")
	}
	if err := process.Resize(terminal.Size{Cols: 200, Rows: 60}); err != nil {
		t.Fatal(err)
	}
	close(releaseEnv)
	readUntil(t, process, "ready")

	wanted := [2]uint32{200, 60}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if server.EffectivePTYSize() == wanted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the remote PTY kept %v; window-changes were %v", server.EffectivePTYSize(), server.Sizes())
}

func TestSetEnvReachesTheRemote(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.SetEnv = []sshclient.EnvVar{{Name: "SSHC", Value: "yes"}}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "ready")
	found := false
	for _, entry := range server.Env() {
		if entry == "SSHC=yes" {
			found = true
		}
	}
	if !found {
		t.Fatalf("env = %#v", server.Env())
	}
}

func TestARemoteCommandRunsInsteadOfAShell(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ran\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.RemoteCommand = "tmux attach"

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "ran")
	if server.Command() != "tmux attach" {
		t.Fatalf("command = %q", server.Command())
	}
	if server.ShellRan() {
		t.Error("a RemoteCommand still started a shell")
	}
}

// プログラムは一つも起動しない。各ホップの SSH チャンネルの上に、次の
// ホップへの TCP を載せるだけである。
func TestProxyJumpReachesTheFinalHostThroughTheFirst(t *testing.T) {
	path, contents, public := keyPair(t)
	inner := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "the far side\r\n") },
	})
	edge := newTestServer(t, serverOptions{
		AcceptKeys:       []ssh.PublicKey{public},
		AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{
			inner.Address(): func() net.Conn { return inner.Dial() },
		},
	})

	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	known := knownHostsLine("["+edge.Host()+"]:"+edge.Port(), edge.HostKey.PublicKey()) +
		knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     auth,
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
	}

	target := targetWith(inner, path)
	target.Jump = []sshclient.Target{targetWith(edge, path)}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "the far side")
	if dialed := edge.Dialed(); len(dialed) != 1 || dialed[0] != inner.Address() {
		t.Fatalf("the first hop dialed %#v", dialed)
	}
}

func TestNestedProxyJumpUsesTheSameFlattenedRouteAsTheResolvedTarget(t *testing.T) {
	path, contents, public := keyPair(t)
	final := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "nested route ready\r\n") },
	})
	bastion := newTestServer(t, serverOptions{
		AcceptKeys:       []ssh.PublicKey{public},
		AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{
			final.Address(): func() net.Conn { return final.Dial() },
		},
	})
	gateway := newTestServer(t, serverOptions{
		AcceptKeys:       []ssh.PublicKey{public},
		AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{
			bastion.Address(): func() net.Conn { return bastion.Dial() },
		},
	})

	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	known := knownHostsLine("["+gateway.Host()+"]:"+gateway.Port(), gateway.HostKey.PublicKey()) +
		knownHostsLine("["+bastion.Host()+"]:"+bastion.Port(), bastion.HostKey.PublicKey()) +
		knownHostsLine("["+final.Host()+"]:"+final.Port(), final.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     auth,
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
		Dial: func(_ context.Context, _, address string) (net.Conn, error) {
			if address != gateway.Address() {
				return nil, errors.New("the first TCP connection bypassed the gateway")
			}
			return gateway.Dial(), nil
		},
	}

	target := targetWith(final, path)
	bastionTarget := targetWith(bastion, path)
	bastionTarget.Alias = "bastion"
	gatewayTarget := targetWith(gateway, path)
	gatewayTarget.Alias = "gateway"
	bastionTarget.Jump = []sshclient.Target{gatewayTarget}
	target.Jump = []sshclient.Target{bastionTarget}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "nested route ready")
	if dialed := gateway.Dialed(); len(dialed) != 1 || dialed[0] != bastion.Address() {
		t.Fatalf("gateway dialed %#v", dialed)
	}
	if dialed := bastion.Dialed(); len(dialed) != 1 || dialed[0] != final.Address() {
		t.Fatalf("bastion dialed %#v", dialed)
	}
}

func TestProxyJumpPasswordPromptAndProgressIdentifyTheHop(t *testing.T) {
	inner := newTestServer(t, serverOptions{Password: "far-password"})
	edge := newTestServer(t, serverOptions{
		Password: "edge-password", AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{inner.Address(): func() net.Conn { return inner.Dial() }},
	})
	known := knownHostsLine("["+edge.Host()+"]:"+edge.Port(), edge.HostKey.PublicKey()) +
		knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())
	dialer := sshclient.Dialer{HostKeys: sshclient.HostKeys{
		Read: func() ([]byte, error) { return []byte(known), nil },
	}}
	target := targetWith(inner)
	target.Alias = "destination"
	jump := targetWith(edge)
	jump.Alias = "mdx-jamstec-1"
	target.Jump = []sshclient.Target{jump}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "Password for ops@"+edge.Address()+" (mdx-jamstec-1): ")
	progressing, ok := process.(terminal.Progressing)
	if !ok {
		t.Fatal("an SSH session did not expose connection progress")
	}
	progress := progressing.ConnectionProgress()
	if progress.Phase != terminal.ConnectionAuthenticating || progress.Alias != "mdx-jamstec-1" ||
		progress.Hop != 1 || progress.Hops != 2 {
		t.Fatalf("progress = %+v, want authentication at the first of two hops", progress)
	}
}

func TestProxyJumpUsesTheSavedPasswordForEachAlias(t *testing.T) {
	inner := newTestServer(t, serverOptions{
		Password: "far-password",
		OnShell:  func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	edge := newTestServer(t, serverOptions{
		Password: "edge-password", AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{inner.Address(): func() net.Conn { return inner.Dial() }},
	})
	known := knownHostsLine("["+edge.Host()+"]:"+edge.Port(), edge.HostKey.PublicKey()) +
		knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())
	var requested []string
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{Password: func(target sshclient.Target) (string, bool) {
			requested = append(requested, target.Alias)
			switch target.Alias {
			case "mdx-jamstec-1":
				return "edge-password", true
			case "destination":
				return "far-password", true
			default:
				return "", false
			}
		}},
		HostKeys:  sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
		Verbosity: func() connectionlog.Level { return connectionlog.Brief },
	}
	target := targetWith(inner)
	target.Alias = "destination"
	jump := targetWith(edge)
	jump.Alias = "mdx-jamstec-1"
	target.Jump = []sshclient.Target{jump}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	readUntil(t, process, "mdx-jamstec-1に接続しました（1/2）")
	readUntil(t, process, "destinationに接続しました（2/2）")
	readUntil(t, process, "ready")
	if !slices.Equal(requested, []string{"mdx-jamstec-1", "destination"}) {
		t.Fatalf("passwords requested for %#v", requested)
	}
}

// 接続できなかった理由は端末に残る。セッションは残す。理由が読めるのは
// そこだけである。
func TestAFailedHandshakeWritesItsReasonToTheTerminal(t *testing.T) {
	path, contents, _ := keyPair(t)
	// サーバーは別の鍵しか受け付けない。
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{newHostKey(t).PublicKey()}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("Open returned an error instead of a session: %v", err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "sshc:")
	if info := process.Wait(); info.Code == 0 {
		t.Fatalf("a failed handshake reported a clean exit: %+v", info)
	}
}

// ホスト鍵が食い違えば接続しない。認証まで進んではならない。
func TestAChangedHostKeyStopsTheConnectionBeforeAuthentication(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	// known_hosts には別の鍵が書いてある。
	other := knownHostsLine("["+server.Host()+"]:"+server.Port(), newHostKey(t).PublicKey())
	dialer := sshclient.Dialer{
		Auth:     auth,
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(other), nil }},
	}

	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	if _, ok := process.(terminal.Readier); !ok {
		t.Fatal("SSH process does not expose asynchronous readiness")
	}
	if readyErr := awaitReady(process); !errors.Is(readyErr, sshclient.ErrHostKeyChanged) {
		t.Fatalf("Ready error = %v, want ErrHostKeyChanged", readyErr)
	}

	seen := readUntil(t, process, "sshc:")
	if !strings.Contains(seen, sshclient.ErrHostKeyChanged.Error()) {
		t.Fatalf("the terminal does not say why: %q", seen)
	}
}

// 未知のホストの問いは端末に出て、結果がそこから戻る。
func TestAnUnknownHostIsAskedThroughTheTerminal(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "accepted\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	added := make(chan knownhosts.Candidate, 1)
	dialer := sshclient.Dialer{
		Auth: auth,
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return nil, nil },
			Add:  func(candidate knownhosts.Candidate) error { added <- candidate; return nil },
		},
	}

	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "Are you sure you want to continue connecting")
	if _, err := process.Write([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, process, "accepted")
	select {
	case candidate := <-added:
		if candidate.Host != server.Host() {
			t.Errorf("the written key names %q", candidate.Host)
		}
	case <-time.After(5 * time.Second):
		t.Error("the accepted key was never written to known_hosts")
	}
}

func TestAnUnreachableAddressFailsWithinItsTimeout(t *testing.T) {
	target := sshclient.Target{
		Alias: "gone", HostName: "203.0.113.10", Port: "22", User: "ops",
		Timeout: 200 * time.Millisecond, Methods: sshclient.DefaultMethods(),
	}
	dialer := sshclient.Dialer{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	drain(process)

	done := make(chan terminal.ExitInfo, 1)
	go func() { done <- process.Wait() }()
	select {
	case info := <-done:
		if info.Code == 0 {
			t.Fatalf("an unreachable address reported a clean exit: %+v", info)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the connection did not give up within its timeout")
	}
}

func TestAHandshakeTheServerNeverAnswersFailsWithinItsTimeout(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	t.Cleanup(func() { _ = serverEnd.Close() })
	go func() { _, _ = io.Copy(io.Discard, serverEnd) }()
	dialer := sshclient.Dialer{Dial: func(context.Context, string, string) (net.Conn, error) {
		return clientEnd, nil
	}}
	target := sshclient.Target{
		Alias: "silent", HostName: "127.0.0.1", Port: "22", User: "ops",
		Timeout: 200 * time.Millisecond, Methods: sshclient.DefaultMethods(),
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	drain(process)

	select {
	case <-process.(terminal.Readier).Ready():
		if readyErr := process.(terminal.Readier).ReadyErr(); !errors.Is(readyErr, context.DeadlineExceeded) {
			t.Fatalf("Ready = %v, want the connect timeout", readyErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the handshake did not give up within its timeout")
	}
}

func TestAPasswordTypedAfterTheConnectTimeoutStillAuthenticates(t *testing.T) {
	const connectTimeout = 300 * time.Millisecond
	// 利用者が ConnectTimeout より長く考えてから答える。
	const thinkingTime = 2 * connectTimeout
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	target := targetWith(server)
	target.Timeout = connectTimeout

	process, err := dialerFor(t, server, sshclient.Auth{}).Open(
		context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	readUntil(t, process, "Password for ")
	drain(process)

	time.Sleep(thinkingTime)
	if _, err := process.Write([]byte("hunter2\r")); err != nil {
		t.Fatal(err)
	}
	if readyErr := awaitReady(process); readyErr != nil {
		t.Fatalf("Ready = %v", readyErr)
	}
}

// 認証中の待ちは、利用者の入力のほかにサーバーの側（プッシュ承認、PAMの遅延）や
// エージェントの側（Touch ID）にもある。OpenSSH と同じく ConnectTimeout では数えない。
func TestAServerThatChecksAPasswordLongerThanTheConnectTimeoutStillAuthenticates(t *testing.T) {
	const connectTimeout = 300 * time.Millisecond
	// サーバーがパスワードを確かめるのに ConnectTimeout より長く掛かる。
	const checkingTime = 2 * connectTimeout
	server := newTestServer(t, serverOptions{
		Password:            "hunter2",
		BeforePasswordReply: func() { time.Sleep(checkingTime) },
	})
	target := targetWith(server)
	target.Timeout = connectTimeout
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }}

	if _, err := dialerFor(t, server, auth).Run(context.Background(), target, sshclient.Command{Line: "true"}); err != nil {
		t.Fatalf("Run = %v", err)
	}
}

// silencingConn は、silence のあとにサーバーから届くものをクライアントへ渡さない。
// 利用者が答えたあとに応答しなくなったサーバーを再現する。
type silencingConn struct {
	net.Conn
	silenced  atomic.Bool
	closed    chan struct{}
	closeOnce sync.Once
}

func newSilencingConn(conn net.Conn) *silencingConn {
	return &silencingConn{Conn: conn, closed: make(chan struct{})}
}

func (c *silencingConn) silence() { c.silenced.Store(true) }

func (c *silencingConn) Read(buffer []byte) (int, error) {
	read, err := c.Conn.Read(buffer)
	if c.silenced.Load() {
		<-c.closed
		return 0, net.ErrClosed
	}
	return read, err
}

func (c *silencingConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// 答えたら同じ長さで数え直す。数え直さなければ、答えたあとに応答しなくなった
// サーバーへの接続は、利用者が閉じるまで終わらない。
func TestAServerThatFallsSilentAfterTheHostKeyIsAcceptedFailsWithinTheConnectTimeout(t *testing.T) {
	const connectTimeout = 300 * time.Millisecond
	// 利用者が ConnectTimeout より長く考えてから答える。
	const thinkingTime = 2 * connectTimeout
	// 数え直した時計が切れるまでの待ちの上限。ConnectTimeout に十分な余裕を足す。
	const giveUpWithin = 10 * connectTimeout
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	dialed := make(chan *silencingConn, 1)
	dialer := sshclient.Dialer{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			transport := newSilencingConn(conn)
			dialed <- transport
			return transport, nil
		},
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return nil, nil },
			Add:  func(knownhosts.Candidate) error { return nil },
		},
	}
	target := targetWith(server)
	target.Timeout = connectTimeout

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	readUntil(t, process, "Are you sure you want to continue connecting")
	drain(process)
	transport := <-dialed

	time.Sleep(thinkingTime)
	ready := process.(terminal.Readier).Ready()
	select {
	case <-ready:
		t.Fatalf("the connection gave up while the user was deciding: %v", process.(terminal.Readier).ReadyErr())
	default:
	}
	transport.silence()
	if _, err := process.Write([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-ready:
		if readyErr := process.(terminal.Readier).ReadyErr(); !errors.Is(readyErr, context.DeadlineExceeded) {
			t.Fatalf("Ready = %v, want the connect timeout", readyErr)
		}
	case <-time.After(giveUpWithin):
		t.Fatal("the connection kept waiting for a silent server after the user answered")
	}
}

func TestClosingASessionCancelsAHandshakeAndClosesItsRawTransport(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	t.Cleanup(func() { _ = serverEnd.Close() })
	transport := &closeObservedConn{Conn: clientEnd, closed: make(chan struct{})}
	dialed := make(chan struct{})
	dialer := sshclient.Dialer{Dial: func(context.Context, string, string) (net.Conn, error) {
		close(dialed)
		return transport, nil
	}}
	target := sshclient.Target{
		Alias: "stalled", HostName: "127.0.0.1", Port: "22", User: "ops",
		Timeout: 5 * time.Second, Methods: sshclient.DefaultMethods(),
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-dialed:
	case <-time.After(time.Second):
		t.Fatal("the handshake never opened its transport")
	}
	if err := process.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	select {
	case <-transport.closed:
	case <-time.After(time.Second):
		t.Fatal("Session.Close did not close the pre-attach raw transport")
	}
}

func TestHangupEndsTheSession(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			// 何も来ないまま待つ。閉じるのはこちら側の仕事である。
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(
		context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, process, "ready")
	drain(process)

	if err := process.Hangup(); err != nil {
		t.Fatal(err)
	}
	done := make(chan terminal.ExitInfo, 1)
	go func() { done <- process.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Hangup did not end the session")
	}
	_ = process.Close()
}

// 接続中の診断が `ssh -v` と同様に端末の stderr へ出力されることを検証する。
func TestTheConnectionLogReachesTheTerminalWhenItIsAsked(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	dialer := dialerFor(t, server, auth)
	dialer.Verbosity = func() connectionlog.Level { return connectionlog.Detailed }
	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	// 深さ 2 では、接続先とSSHハンドシェイクの完了を表示する。
	readUntil(t, process, "へ接続します")
	readUntil(t, process, "認証方式を試します：publickey")
	readUntil(t, process, "SSHハンドシェイクが完了しました")
	readUntil(t, process, "ready")
}

func TestFullConnectionLogExplainsAnAutomaticallyAnsweredEchoedTOTP(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Keyboard:     map[string]string{"Verification code: ": "123456"},
		KeyboardEcho: true, KeyboardName: "tsukuba",
		OnShell: func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{TOTP: func(_ sshclient.Target, question string) (string, bool) {
		return "123456", question == "Verification code: "
	}}
	dialer := dialerFor(t, server, auth)
	dialer.Verbosity = func() connectionlog.Level { return connectionlog.Full }
	process, err := dialer.Open(context.Background(), targetWith(server), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	seen := readUntil(t, process, "ready")
	for _, want := range []string{
		"接続ログ：すべて（-vvv）",
		"認証方式を試します：keyboard-interactive",
		"keyboard-interactiveのプロンプト1/1：Verification code:（入力表示：あり）",
		"保存済みTOTPをbastionの認証コードのプロンプトへ入力しました。",
		"認証方式keyboard-interactiveで認証されました。",
	} {
		if !strings.Contains(seen, want) {
			t.Errorf("full connection log did not contain %q:\n%s", want, seen)
		}
	}
	if strings.Contains(seen, "123456") {
		t.Fatalf("full connection log exposed the TOTP code:\n%s", seen)
	}
}

// 既定は無言である。毎回この量が流れると、シェルの最初の一画面が押し流される。
func TestNothingIsWrittenWhenTheLogWasNotAskedFor(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell:    func(channel ssh.Channel) { _, _ = io.WriteString(channel, "ready\r\n") },
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}

	process, err := dialerFor(t, server, auth).Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	// 最初に届くのがシェルの出力である。途中経過が混ざっていれば、
	// この読み取りはそれを先に見る。
	seen := readUntil(t, process, "ready")
	if strings.Contains(seen, "[sshc]") {
		t.Errorf("何も頼んでいないのに途中経過が出た:\n%s", seen)
	}
}
