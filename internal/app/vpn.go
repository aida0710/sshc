package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

// vpnStateDirectory は、中継のソケットを置く engine 専用ディレクトリである。
// ワークスペースの中にあるが、同期からは外してある。socket は運べない。
const vpnStateDirectory = "sshc/vpn"

const (
	// vpnIdleTimeout は、接続が一本も通っていない経路を畳むまでの長さである。
	// 接続と接続のあいだに張り直させない程度には長く、忘れたコンテナが一日
	// 動き続けない程度には短くする。
	vpnIdleTimeout = 10 * time.Minute
	// vpnSweepInterval は、無操作の経路を探しに行く間隔である。
	vpnSweepInterval = time.Minute
	// vpnStopTimeout は、engine を終えるときに経路を畳むのを待つ上限である。
	// docker が応えない機械で、終了そのものが止まらないようにする。
	vpnStopTimeout = 30 * time.Second
)

// superviseVPNSessions は、engine が動いているあいだ経路の寿命を見る。
//
// 起動したときに、前回の engine が残したコンテナを回収する。引き継がないのは、
// そのコンテナがどの設定で経路を張ったのかを確かめられないからである。その後は、
// 誰も通っていない経路を畳み続ける。
func superviseVPNSessions(ctx context.Context, sessions *vpn.Manager, logger *slog.Logger) {
	if err := sessions.DiscardOrphans(ctx); err != nil && logger != nil &&
		!errors.Is(err, vpn.ErrDockerMissing) {
		logger.Warn("discard vpn sessions left by a previous engine", "error", err)
	}
	ticker := time.NewTicker(vpnSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessions.StopIdle(ctx, vpnIdleTimeout)
		}
	}
}

// vpnRoute は、プロファイル名から設定と秘密を集め、その経路で接続先へ繋ぐ。
func vpnRoute(
	profiles *vpnprofile.Service,
	sessions *vpn.Manager,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, name, address string) (net.Conn, error) {
		profile, secrets, err := profiles.Route(name)
		if err != nil {
			return nil, err
		}
		if !profile.Reaches(address) {
			return nil, fmt.Errorf("%w: profile %s targets %s", vpn.ErrTargetMismatch, name, profile.Target.Address())
		}
		return sessions.Dial(ctx, profile, secrets)
	}
}
