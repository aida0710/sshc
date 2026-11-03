package sshclient

import (
	"context"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/textencoding"
)

// Connection is an authenticated SSH transport without a terminal channel.
// Subsystems such as SFTP can open their own SSH channels on it. It is
// deliberately non-interactive: stored credentials and known hosts are used,
// while a prompt that would need a terminal makes the connection fail. The one
// exception is a host key known_hosts does not have, which a person watching
// the connection may be asked about (ConnectionObserver.ConfirmHostKey).
type Connection struct {
	client *ssh.Client
	// encoding は、Run が運ぶコマンドの入出力の文字コードである。
	encoding textencoding.Name
	closers  []io.Closer
	done     chan struct{}
	once     sync.Once
	err      error
}

// Connect authenticates one target and returns a transport owned by the
// caller. Close must be called when its subsystem operation is complete.
func (d Dialer) Connect(ctx context.Context, target Target) (*Connection, error) {
	route, prompt := subsystemRoute(ctx, target)
	client, closers, err := d.chain(ctx, route, prompt, d.observingTracer(ctx))
	if err != nil {
		return nil, err
	}
	connection := &Connection{
		client: client, encoding: route.Encoding, closers: append(closers, client), done: make(chan struct{}),
	}
	if keepAlive := keepAliveLoop(client, keepAliveSettings{interval: route.KeepAlive, count: route.KeepAliveMax, done: connection.done}); keepAlive != nil {
		go keepAlive()
	}
	return connection, nil
}

// subsystemRoute は、端末の無い接続の経路と、その途中で尋ねる手段を決める。
//
// 接続を見ている人が未知のホスト鍵に答えられるとき（SFTP のタブが最初の一覧を取る接続）は、
// ターミナルと同じく各段の StrictHostKeyChecking に従い、ask ではその人に尋ねる。認証の
// 入力（パスフレーズ、パスワード、ワンタイムパスワード）は尋ねず、保存済みの資格情報だけを使う。
//
// 答えられる人がいなければ、ホスト鍵を尋ねる先が無い。保存した接続先が普段は対話の確認を
// 許していても、どの段でも未知の鍵を断る。最後の接続先だけにすると、ProxyJump の段が
// 未知の鍵を自動で登録してしまう。
func subsystemRoute(ctx context.Context, target Target) (Target, Prompter) {
	observer, _ := connectionObserverIn(ctx)
	if observer.ConfirmHostKey == nil {
		return requireKnownHosts(target), noPrompt
	}
	return target, hostKeyQuestionPrompter{confirm: func(question string) (bool, error) {
		return observer.ConfirmHostKey(ctx, question)
	}}
}

func requireKnownHosts(target Target) Target {
	// StrictHostKeyCheckingの強制は、非対話処理が資格情報を未知のhostへ送らないための
	// 安全側の変更である。保存時に確認した接続経路そのものは変わっていないので、
	// password bindingには強制前の値を使う。そうしないと、この関数自身が変更した
	// Strictだけを理由に、正しい保存passwordを拒否してしまう。
	target.authenticationBindingOverride = target.AuthenticationBinding()
	target.Strict = "yes"
	target.Jump = append([]Target(nil), target.Jump...)
	for index := range target.Jump {
		target.Jump[index] = requireKnownHosts(target.Jump[index])
	}
	return target
}

// Client exposes the authenticated transport to SSH channel protocols.
func (c *Connection) Client() *ssh.Client { return c.client }

// Run は、この輸送の上でコマンドをひとつ走らせる。Dialer.Run と違い輸送は開いたまま
// にする。ctx の取り消しか Limit の期限で打ち切るときも、閉じるのはそのチャンネルだけである。
func (c *Connection) Run(ctx context.Context, command Command) (Output, error) {
	return runCommand(ctx, commandRun{
		client: c.client, encoding: c.encoding, command: command, started: time.Now(), abandon: func() {},
	})
}

// Ping sends one keepalive and reports whether the SSH server answers within
// limit: context.DeadlineExceeded when it does not, context.Canceled after Close.
func (c *Connection) Ping(limit time.Duration) error {
	return keepAliveReply(c.client, limit, c.done)
}

// Wait blocks until the final transport ends, whether the peer or Close ended it.
func (c *Connection) Wait() error { return c.client.Wait() }

// Close releases the final transport and every ProxyJump hop, deepest first.
func (c *Connection) Close() error {
	c.once.Do(func() {
		close(c.done)
		for index := len(c.closers) - 1; index >= 0; index-- {
			if err := c.closers[index].Close(); err != nil && c.err == nil {
				c.err = err
			}
		}
	})
	return c.err
}
