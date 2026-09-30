package app

import (
	"context"
	"errors"
	"testing"

	"sshc/internal/application"
	"sshc/internal/sshclient"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
	"sshc/internal/vpnrefusal"
)

// missingProfiles は、どの名前のプロファイルも無いと答える設定である。経路の起動は、
// 設定を読んだところで断られるので、ほかの操作は呼ばれない。
type missingProfiles struct{ vpnprofile.Configuration }

func (missingProfiles) StoredVPNProfile(string) (application.VPNProfile, error) {
	return application.VPNProfile{}, application.ErrUnknownVPNProfile
}

// 経路を断った理由は、Terminal の接続ログに出す文にし、元の失敗も辿れるようにする。
func TestARefusedRouteIsExplainedInTheConnectionLog(t *testing.T) {
	route := vpnRoute(vpnprofile.New(vpnprofile.Dependencies{Configuration: missingProfiles{}}),
		vpn.New(t.TempDir(), 1000, nil))

	_, err := route(context.Background(), "lab", "10.9.9.1:22")

	var explained *sshclient.ExplainedError
	if !errors.As(err, &explained) {
		t.Fatalf("err = %v, want an explained error", err)
	}
	if want := vpnrefusal.Sentence(vpnrefusal.Refusal{Code: vpnrefusal.CodeProfileUnknown}); explained.Sentence != want {
		t.Fatalf("sentence = %q, want %q", explained.Sentence, want)
	}
	if !errors.Is(err, application.ErrUnknownVPNProfile) {
		t.Fatalf("元の失敗を辿れない: %v", err)
	}
}
