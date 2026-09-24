package main

import (
	"context"
	"time"

	"sshc/internal/connectionlog"
	"sshc/internal/httpserver"
	"sshc/internal/vpn"
)

// VPN 経路を起こすのを待つあいだ、時間のかかる段階と、利用者が何かをする段階に
// 入ったことを、接続ログの設定に関係なく知らせる。
//
// Terminal の接続は sshcエンジンの中で経路を用意し、同じ文（vpn.StartPhase.Notice）を
// その場で書く。CLI の接続は sshcエンジンに経路を頼んで待つだけなので、こちらから
// 段階を見に行って書く。

// phasePollInterval は、sshcエンジンの段階を見に行く間隔である。知らせる段階は
// 数秒から数分続くので、この間隔で見落とすことはない。
const phasePollInterval = 500 * time.Millisecond

// phaseWatch は、段階を見に行く先である。
type phaseWatch struct {
	engine   *engineAPI
	profile  string
	interval time.Duration
}

// announcePhases は、ctx が終わるまで、経路の段階を見て、知らせる文のある段階に
// 入ったら1度だけ書く。段階を読めなかったときは、黙って次を待つ。知らせは接続の
// 成否を変えない。
func announcePhases(ctx context.Context, watch phaseWatch) {
	ticker := time.NewTicker(watch.interval)
	defer ticker.Stop()
	announced := map[vpn.StartPhase]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var overview httpserver.VPNOverview
		if err := watch.engine.getJSON(ctx, "/api/v1/vpn", &overview); err != nil {
			continue
		}
		phase := routePhase(overview, watch.profile)
		if notice := phase.Notice(); notice != "" && !announced[phase] {
			announced[phase] = true
			connectionlog.Say(ctx, connectionlog.Notice, "%s", notice)
		}
	}
}

// routePhase は、一覧からそのプロファイルの経路の段階を返す。
func routePhase(overview httpserver.VPNOverview, profile string) vpn.StartPhase {
	for _, status := range overview.Profiles {
		if status.Profile.Name == profile {
			return status.Phase
		}
	}
	return ""
}
