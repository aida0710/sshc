package app

import (
	"testing"

	"sshc/internal/application"
	"sshc/internal/effective"
	"sshc/internal/textencoding"
)

func sessionTargetParts() sshParts {
	return sshParts{
		resolve: func(alias string) (effective.Values, error) {
			return effective.Values{Entries: map[string][]string{"hostname": {"from-" + alias}, "user": {"from-config"}}}, nil
		},
		encoding: func(string) (textencoding.Name, error) { return textencoding.UTF8, nil },
		vpnProfile: func(alias string) (application.AttachedVPNProfile, error) {
			return application.AttachedVPNProfile{Name: "vpn-of-" + alias, ID: "id"}, nil
		},
	}
}

func TestASessionTargetIsAConnectionOrADestinationThatWasNotSaved(t *testing.T) {
	parts := sessionTargetParts()
	saved, err := parts.sessionTarget("web")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Direct || saved.Alias != "web" || saved.HostName != "from-web" || saved.User != "from-config" {
		t.Fatalf("saved = %+v", saved)
	}

	direct, err := parts.sessionTarget("deploy@web:2222")
	if err != nil {
		t.Fatal(err)
	}
	if !direct.Direct || direct.Alias != "web" || direct.User != "deploy" || direct.Port != "2222" {
		t.Fatalf("direct = %+v", direct)
	}
	// 接続名と同じ名前を打ったら、ssh と同じくその接続の設定（VPN も）を通る。
	if direct.VPN != "vpn-of-web" {
		t.Fatalf("direct.VPN = %q", direct.VPN)
	}
}
