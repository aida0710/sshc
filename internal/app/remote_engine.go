package app

import (
	"context"
	"net"
	"time"

	"sshc/internal/remoteengine"
	"sshc/internal/sshclient"
)

// remoteEngineKeepAlive は、ServerAliveInterval が書かれていない接続先へ、中継の SSH
// 接続が送る keepalive の間隔である。中継の接続は、利用者が離れているあいだ何時間も
// 何も流さない。その間に NAT や VPN が黙って経路を捨てると、keepalive が無ければ接続は
// 切れたことに気付かず、戻ってきた最初の要求が TCP のタイムアウト（数分）まで止まる。
// ServerAliveCountMax（既定 3）回続けて応答が無ければ接続を閉じ、次の要求で繋ぎ直す。
const remoteEngineKeepAlive = 30 * time.Second

// remoteEngineLink は、接続先のエンジンへの中継が使う SSH の接続を開く。
//
// SFTP と同じく非対話である。未知のホスト鍵や、保存していないパスフレーズを尋ねる
// 端末が無いので、尋ねる必要がある接続は失敗する。
func (p sshParts) remoteEngineLink(ctx context.Context, alias string) (remoteengine.Link, error) {
	target, err := p.target(alias)
	if err != nil {
		return nil, err
	}
	if target.KeepAlive <= 0 {
		target.KeepAlive = remoteEngineKeepAlive
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
	return remoteengine.RunResult{Stdout: output.Stdout, Stderr: output.Stderr, ExitCode: output.ExitCode}, err
}

func (l sshRemoteEngineLink) Wait() error  { return l.connection.Wait() }
func (l sshRemoteEngineLink) Close() error { return l.connection.Close() }
