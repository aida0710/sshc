package sshclient

import (
	"context"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

// Connection is an authenticated SSH transport without a terminal channel.
// Subsystems such as SFTP can open their own SSH channels on it. It is
// deliberately non-interactive: stored credentials and known hosts are used,
// while a prompt that would need a terminal makes the connection fail.
type Connection struct {
	client  *ssh.Client
	closers []io.Closer
	done    chan struct{}
	once    sync.Once
	err     error
}

// Connect authenticates one target and returns a transport owned by the
// caller. Close must be called when its subsystem operation is complete.
func (d Dialer) Connect(ctx context.Context, target Target) (*Connection, error) {
	// A subsystem has no terminal on which to ask whether an unknown host key
	// should be trusted. Force every stage of the non-interactive path to fail
	// closed even when the saved target would normally allow an interactive
	// confirmation. Applying this only to the final host leaves ProxyJump hops
	// able to auto-register an unknown key before the subsystem reaches it.
	strict := requireKnownHosts(target)
	client, closers, err := d.chain(ctx, strict, noPrompt, nil)
	if err != nil {
		return nil, err
	}
	connection := &Connection{
		client: client, closers: append(closers, client), done: make(chan struct{}),
	}
	if keepAlive := keepAliveLoop(client, strict.KeepAlive, strict.KeepAliveMax, connection.done); keepAlive != nil {
		go keepAlive()
	}
	return connection, nil
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
