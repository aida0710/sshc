package vpnprofile_test

import (
	"errors"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// testOpenVPNConfig は、remote をひとつ持つクライアントの設定ファイルである。
const testOpenVPNConfig = "client\ndev tun\nremote vpn.example.jp 1194\n"

func openVPNProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "lab", Backend: vpn.OpenVPN,
		OpenVPN: &application.OpenVPNProfile{Servers: []string{"vpn.example.jp"}},
	}
}

// 設定ファイルを送らずにユーザー名とパスワードを足すと、保存済みの設定ファイルのまま残す。
func TestAddingOpenVPNCredentialsKeepsTheStoredConfig(t *testing.T) {
	f := newFixture(t)
	f.create(t, openVPNProfile(), vpn.SecretsDocument{OpenVPNConfig: testOpenVPNConfig})
	updated := openVPNProfile()
	updated.OpenVPN.Username = "fixture"

	if err := f.profiles.Update(updated, &vpn.SecretsDocument{OpenVPNPassword: "a password"}); err != nil {
		t.Fatalf("Update = %v", err)
	}

	got := f.storedSecrets(t, "lab")
	if got.OpenVPNConfig != testOpenVPNConfig || got.OpenVPNPassword != "a password" {
		t.Fatalf("stored = %+v", got)
	}
}

// サーバーの並びが保存済みの設定ファイルの remote と合わなければ、何も変えずに断る。
// 一覧に出るサーバーと、実際に繋ぐサーバーを食い違わせない。
func TestServersThatDisagreeWithTheStoredConfigAreRefused(t *testing.T) {
	f := newFixture(t)
	f.create(t, openVPNProfile(), vpn.SecretsDocument{OpenVPNConfig: testOpenVPNConfig})
	updated := openVPNProfile()
	updated.OpenVPN.Servers = []string{"other.example.jp"}

	err := f.profiles.Update(updated, nil)

	var fieldError *vpn.FieldError
	if !errors.As(err, &fieldError) || fieldError.Field != "openvpn.servers" || fieldError.Reason != vpn.ReasonConfigMismatch {
		t.Fatalf("Update = %v, want config_mismatch", err)
	}
	profiles, _ := f.config.VPNProfiles()
	if len(profiles) != 1 || profiles[0].OpenVPN.Servers[0] != "vpn.example.jp" {
		t.Fatalf("profiles = %+v", profiles)
	}
}
