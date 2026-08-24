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
	once    sync.Once
	err     error
}

// Connect authenticates one target and returns a transport owned by the
// caller. Close must be called when its subsystem operation is complete.
func (d Dialer) Connect(ctx context.Context, target Target) (*Connection, error) {
	// A subsystem has no terminal on which to ask whether an unknown host key
	// should be trusted. Force the non-interactive path to fail closed even when
	// the saved target would normally allow an interactive confirmation.
	strict := target
	strict.Strict = "yes"
	client, closers, err := d.chain(ctx, strict, nil, nil)
	if err != nil {
		return nil, err
	}
	return &Connection{client: client, closers: append(closers, client)}, nil
}

// Client exposes the authenticated transport to SSH channel protocols.
func (c *Connection) Client() *ssh.Client { return c.client }

// Close releases the final transport and every ProxyJump hop, deepest first.
func (c *Connection) Close() error {
	c.once.Do(func() {
		for index := len(c.closers) - 1; index >= 0; index-- {
			if err := c.closers[index].Close(); err != nil && c.err == nil {
				c.err = err
			}
		}
	})
	return c.err
}
