package app

import (
	"context"
	"net"
	"time"

	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
)

// remoteEngineLink は、接続先のエンジンへの中継が使う SSH の接続を開く。
//
// SFTP と同じく非対話である。未知のホスト鍵や、保存していないパスフレーズを尋ねる
// 端末が無いので、尋ねる必要がある接続は失敗する。
func (p sshParts) remoteEngineLink(ctx context.Context, alias string) (remoteengine.Link, error) {
	target, err := p.target(alias)
	if err != nil {
		return nil, err
	}
	connection, err := p.dialer.Connect(ctx, target)
	if err != nil {
		return nil, err
	}
	return sshRemoteEngineLink{connection: connection}, nil
}

// sshRemoteEngineLink は、sshclient.Connection を remoteengine.Link として使う。
type sshRemoteEngineLink struct {
	connection *sshclient.Connection
}

func (l sshRemoteEngineLink) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return l.connection.Client().DialContext(ctx, network, address)
}

func (l sshRemoteEngineLink) Run(ctx context.Context, command string, limit time.Duration) (remoteengine.RunResult, error) {
	output, err := l.connection.Run(ctx, sshclient.Command{Line: command, Limit: limit})
	return remoteengine.RunResult{Stdout: output.Stdout, ExitCode: output.ExitCode}, err
}

func (l sshRemoteEngineLink) Wait() error  { return l.connection.Wait() }
func (l sshRemoteEngineLink) Close() error { return l.connection.Close() }
