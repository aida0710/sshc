package sshclient_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/sshclient"
)

// Short intervals exercise keepalive without slowing down the local SSH fixture.
const diagnosticKeepAliveInterval = 20 * time.Millisecond

func TestTheFullLogExplainsKeyRejectionAndPasswordSuccess(t *testing.T) {
	path, contents, _ := keyPair(t)
	_, _, accepted := keyPair(t)
	const password = "diagnostic-test-password"
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{accepted}, Password: password, ExitCode: 7,
	})
	dialer := dialerFor(t, server, sshclient.Auth{
		ReadFile: func(string) ([]byte, error) { return contents, nil },
		Password: func(sshclient.Target) (string, bool) { return password, true },
	})
	process := openWithLog(t, dialer, targetWith(server, path), connectionlog.Full)
	output, err := io.ReadAll(process)
	if err != nil {
		t.Fatal(err)
	}
	info := process.Wait()
	seen := string(output) + info.Notice
	expectLines(t, seen,
		"提示する鍵交換アルゴリズム：", "提示する暗号：", "提示するMAC：",
		"認証前のサーバーのSSHバージョン：SSH-2.0-Go",
		"採用された鍵交換：", "クライアント → サーバー：暗号", "サーバー → クライアント：暗号",
		"サーバーが受け付ける認証方式：", "成功しなかった認証方式：none, publickey",
		"認証方式passwordで認証されました。",
		"SSHセッションチャンネルが開きました", "PTY要求が受け入れられました", "起動要求が受け入れられました",
		"SSHセッションが終了しました：コード7",
	)
	if strings.Contains(seen, password) {
		t.Fatal("the diagnostic log exposed a password")
	}
	if info.Code != 7 {
		t.Fatal("logging changed the remote exit status")
	}
}

// シェルが終わったあとの接続ログは、出力に書かずに Notice で返す。engine がモードを
// 戻してから書くので、終わったプログラムの代替画面に隠れない。
func TestTheLogOfHowTheShellEndedComesWithTheNoticeInsteadOfTheOutput(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{ExitCode: 7})
	process := openWithLog(t, dialer, target, connectionlog.Detailed)
	output, err := io.ReadAll(process)
	if err != nil {
		t.Fatal(err)
	}
	info := process.Wait()
	const ended = "SSHセッションが終了しました：コード7"
	if strings.Contains(string(output), ended) {
		t.Fatalf("output = %q; the line was written where the program's alternate screen hides it", output)
	}
	if !strings.Contains(info.Notice, ended) {
		t.Fatalf("notice = %q, want %q", info.Notice, ended)
	}
}

func TestTheKeepAliveLogExplainsWhyAnUnresponsiveConnectionWasClosed(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{
		IgnoreKeepAlives: true,
		OnShell:          func(channel ssh.Channel) { _, _ = io.Copy(io.Discard, channel) },
	})
	target.KeepAlive, target.KeepAliveMax = diagnosticKeepAliveInterval, 2
	process := openWithLog(t, dialer, target, connectionlog.Full)
	output, err := io.ReadAll(process)
	if err != nil {
		t.Fatal(err)
	}
	info := process.Wait()
	expectLines(t, string(output), "keepaliveを送信します。", "連続1/2回", "連続2/2回", "context deadline exceeded")

	// 切断した理由は、出力に書かずに、輸送が落ちたことの文より前に置いて返す。
	// 出力に書くと、終わったプログラムの代替画面に隠れる。
	const limitReached = "keepaliveの連続失敗が上限に達したため、SSH接続を切断します。"
	if strings.Contains(string(output), limitReached) {
		t.Fatalf("output = %q; the reason was written where the program's alternate screen hides it", output)
	}
	if !info.TransportLost || !strings.HasPrefix(strings.TrimLeft(info.Notice, "\r\n"), "[sshc][debug1] "+limitReached) {
		t.Fatalf("exit = %+v, want a lost transport whose notice begins with %q", info, limitReached)
	}
}

// This writer deliberately relies on Stream to serialize diagnostic and remote
// stderr writes. Reading it after Stream returns also verifies keepalive stopped.
type keepAliveLog struct {
	bytes.Buffer
	replied chan struct{}
	once    sync.Once
}

func (log *keepAliveLog) Write(contents []byte) (int, error) {
	written, err := log.Buffer.Write(contents)
	if bytes.Contains(contents, []byte("keepaliveの応答を受信しました")) {
		log.once.Do(func() { close(log.replied) })
	}
	return written, err
}

func TestStreamLogsKeepAliveRepliesAlongsideRemoteStderr(t *testing.T) {
	log := &keepAliveLog{replied: make(chan struct{})}
	_, dialer, target := streamSetup(t, serverOptions{
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel.Stderr(), "remote diagnostic\n")
			select {
			case <-log.replied:
			case <-t.Context().Done():
			}
		},
	})
	target.KeepAlive = diagnosticKeepAliveInterval
	dialer.Verbosity = func() connectionlog.Level { return connectionlog.Full }
	// Bound failures of the fixture; a successful keepalive releases it directly.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	code, err := dialer.Stream(ctx, target, "fixture", sshclient.Streams{Out: io.Discard, Err: log})
	if err != nil || code != 0 {
		t.Fatalf("Stream = %d, %v", code, err)
	}
	expectLines(t, log.String(), "remote diagnostic", "keepaliveの応答を受信しました", "SSHセッションが終了しました：コード0")
}
