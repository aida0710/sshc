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
	process := openWithLog(t, dialer, targetWith(server, path), sshclient.Full)
	output, err := io.ReadAll(process)
	if err != nil {
		t.Fatal(err)
	}
	seen := string(output)
	expectLines(t, seen,
		"名乗る鍵交換アルゴリズム：", "名乗る暗号：", "名乗るMAC：",
		"認証前のサーバーのSSHバージョン：SSH-2.0-Go",
		"採用された鍵交換：", "クライアント → サーバー：暗号", "サーバー → クライアント：暗号",
		"サーバーが受け付ける認証方式：", "成功しなかった認証方式：none, publickey",
		"認証方式 password で認証されました。",
		"SSHセッションチャンネルが開きました", "PTY要求が受け入れられました", "起動要求が受け入れられました",
		"SSHセッションが終了しました：コード 7",
	)
	if strings.Contains(seen, password) {
		t.Fatal("the diagnostic log exposed a password")
	}
	if process.Wait().Code != 7 {
		t.Fatal("logging changed the remote exit status")
	}
}

func TestTheKeepAliveLogExplainsWhyAnUnresponsiveConnectionWasClosed(t *testing.T) {
	_, dialer, target := streamSetup(t, serverOptions{
		IgnoreKeepAlives: true,
		OnShell:          func(channel ssh.Channel) { _, _ = io.Copy(io.Discard, channel) },
	})
	target.KeepAlive, target.KeepAliveMax = diagnosticKeepAliveInterval, 2
	process := openWithLog(t, dialer, target, sshclient.Full)
	seen := readUntil(t, process, "keepaliveの連続失敗が上限に達したため、SSH接続を切断します。")
	expectLines(t, seen, "keepaliveを送信します。", "連続 1/2 回", "連続 2/2 回", "context deadline exceeded")
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
	dialer.Verbosity = func() sshclient.Verbosity { return sshclient.Full }
	// Bound failures of the fixture; a successful keepalive releases it directly.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	code, err := dialer.Stream(ctx, target, "fixture", sshclient.Streams{Out: io.Discard, Err: log})
	if err != nil || code != 0 {
		t.Fatalf("Stream = %d, %v", code, err)
	}
	expectLines(t, log.String(), "remote diagnostic", "keepaliveの応答を受信しました", "SSHセッションが終了しました：コード 0")
}
