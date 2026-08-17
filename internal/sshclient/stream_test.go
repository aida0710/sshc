package sshclient_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
)

// streamSetup は、この検査が繰り返す組み立てをひとつにまとめる。
//
// **鍵はサーバーより先に作る。** 受け付ける鍵は ssh.ServerConfig を組んだ時点で
// 決まるので、あとから足しても認証は通らない——握手そのものが失敗する。
func streamSetup(
	t *testing.T, options serverOptions,
) (*testServer, sshclient.Dialer, sshclient.Target) {
	t.Helper()
	path, signer := writeKey(t, t.TempDir(), "id_ed25519", nil)
	options.AcceptKeys = append(options.AcceptKeys, signer.PublicKey())
	server := newTestServer(t, options)
	return server, dialerFor(t, server, sshclient.Auth{}), targetWith(server, path)
}

// **stdout と stderr は分かれて届く。** 混ざれば、出力を集めた側はコマンドの
// 答えと診断を区別できない。対話セッションがこれを一本に畳んでいるのは端末が
// ひとつだからで、端末が無いここではその理由が無い。
func TestStreamKeepsTheTwoOutputsApart(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "this is the answer\n")
			_, _ = io.WriteString(channel.Stderr(), "this is the diagnosis\n")
		},
	})
	var out, errOut bytes.Buffer

	code, err := dialer.Stream(context.Background(), target, "echo",
		sshclient.Streams{Out: &out, Err: &errOut})

	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if out.String() != "this is the answer\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errOut.String() != "this is the diagnosis\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}

// **終了コードは結果であって失敗ではない。** 相手が答えたのだから、その答えを
// そのまま返す。error にしてしまうと、呼び出し側は「走らなかった」と「走って
// 失敗した」を区別できない。
func TestStreamReportsTheRemoteExitStatusWithoutCallingItAnError(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{ExitCode: 3})

	code, err := dialer.Stream(context.Background(), target, "false",
		sshclient.Streams{Out: io.Discard, Err: io.Discard})

	if err != nil {
		t.Fatalf("a command that exited 3 was reported as a failure: %v", err)
	}
	if code != 3 {
		t.Errorf("exit = %d, want 3", code)
	}
}

// **相手のシェルが受け取るのは、渡した一本の文字列である。** こちらで引用を
// 付け直さないので、サーバーに届く文字列は打たれたものと同じでなければならない。
func TestStreamHandsTheCommandOverUnchanged(t *testing.T) {
	server, dialer, target := streamSetup(t, serverOptions{})
	const command = `go test ./... -run 'Test[A-Z]' 2>&1`

	if _, err := dialer.Stream(context.Background(), target, command,
		sshclient.Streams{Out: io.Discard, Err: io.Discard}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if got := server.Command(); got != command {
		t.Errorf("the server was asked to run %q, want %q", got, command)
	}
}

// **端末を要求しない。** 要求すれば相手の出力に画面制御が混ざり、集めた側が
// 読めなくなる。
func TestStreamNeverAsksForATerminal(t *testing.T) {
	server, dialer, target := streamSetup(t, serverOptions{})

	if _, err := dialer.Stream(context.Background(), target, "hostname",
		sshclient.Streams{Out: io.Discard, Err: io.Discard}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if term, _ := server.PTY(); term != "" {
		t.Errorf("a pseudo terminal was requested with TERM=%q", term)
	}
}

// **未知のホスト鍵を黙って受け入れない。** 尋ねる相手が居ない実行が、信頼を
// 増やしてはならない。
func TestStreamRefusesAnUnknownHostInsteadOfTrustingIt(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{})
	// 覚えていない状態にする。設定が accept-new でも、この経路は yes で読む。
	target.Strict = "accept-new"
	dialer.HostKeys = sshclient.HostKeys{Read: func() ([]byte, error) { return nil, nil }}

	_, err := dialer.Stream(context.Background(), target, "hostname",
		sshclient.Streams{Out: io.Discard, Err: io.Discard})

	if !errors.Is(err, sshclient.ErrHostKeyUnknown) {
		t.Fatalf("err = %v, want ErrHostKeyUnknown", err)
	}
}

// **相手が終わるのを永久には待たない。** ctx が終わればセッションを閉じる。
// 閉じなければ Ctrl-C を押した人が待たされ続ける。
func TestStreamStopsWhenTheContextIsDone(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	_, dialer, target := streamSetup(t, serverOptions{
		OnShell: func(ssh.Channel) {
			// 何も書かず、終わらない相手を演じる。
			<-release
		},
	})
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan error, 1)
	go func() {
		_, err := dialer.Stream(ctx, target, "sleep forever",
			sshclient.Streams{Out: io.Discard, Err: io.Discard})
		finished <- err
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled stream never returned")
	}
}

// 走らせるものが無いなら、繋ぐ前に断る。**空のコマンドはシェルを開く指示では
// ない**——この入口は端末を開かないので、それは何も起きないことになる。
func TestStreamRefusesAnEmptyCommandBeforeConnecting(t *testing.T) {
	server, dialer, target := streamSetup(t, serverOptions{})

	code, err := dialer.Stream(context.Background(), target, "",
		sshclient.Streams{Out: io.Discard, Err: io.Discard})

	if err == nil {
		t.Fatal("an empty command was accepted")
	}
	if code != sshclient.RemoteFailureExit {
		t.Errorf("exit = %d, want %d", code, sshclient.RemoteFailureExit)
	}
	if server.ShellRan() {
		t.Error("the server ran something for an empty command")
	}
}

// **標準入力は相手のコマンドのものである。** 問いの答えを読むためにそこから
// 取れば、コマンドへ渡すはずのものを奪う。
func TestStreamForwardsStandardInputToTheCommand(t *testing.T) {
	received := make(chan string, 1)
	_, dialer, target := streamSetup(t, serverOptions{
		OnShell: func(channel ssh.Channel) {
			contents, _ := io.ReadAll(channel)
			received <- string(contents)
		},
	})

	code, err := dialer.Stream(context.Background(), target, "cat", sshclient.Streams{
		In: strings.NewReader("fed through the pipe"), Out: io.Discard, Err: io.Discard,
	})

	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	select {
	case got := <-received:
		if got != "fed through the pipe" {
			t.Errorf("the command read %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the command never received the input")
	}
}

// **設定した ServerAliveInterval は、この入口でも効く。** 対話セッションだけが
// 尊重していて、長く黙って走るコマンドの側が無視していた——途中の機器に接続を
// 捨てられて困るのは、むしろこちらである。
func TestStreamSendsTheKeepAlivesTheConfigurationAsksFor(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	server, dialer, target := streamSetup(t, serverOptions{
		OnShell: func(ssh.Channel) { <-release },
	})
	target.KeepAlive = 50 * time.Millisecond
	target.KeepAliveMax = 100
	ctx, cancel := context.WithCancel(context.Background())

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = dialer.Stream(ctx, target, "sleep forever",
			sshclient.Streams{Out: io.Discard, Err: io.Discard})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && server.KeepAlives() < 3 {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-finished

	if got := server.KeepAlives(); got < 3 {
		t.Errorf("the server saw %d keepalives, want the configured interval to keep sending them", got)
	}
}

// 設定していないなら送らない。**既定を作らない** —— OpenSSH も既定では送らず、
// ここで勝手に送り始めると、設定を読んだ人の予想と食い違う。
func TestStreamSendsNoKeepAlivesWithoutAnInterval(t *testing.T) {
	server, dialer, target := streamSetup(t, serverOptions{})

	if _, err := dialer.Stream(context.Background(), target, "hostname",
		sshclient.Streams{Out: io.Discard, Err: io.Discard}); err != nil {
		t.Fatalf("stream: %v", err)
	}

	if got := server.KeepAlives(); got != 0 {
		t.Errorf("the server saw %d keepalives without ServerAliveInterval", got)
	}
}
