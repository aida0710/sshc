package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// testOpenVPNConfig は、ユーザー名とパスワードを求める、クライアントの設定ファイルである。
const testOpenVPNConfig = "client\ndev tun\nremote vpn.example.jp 1194\nremote backup.example.jp 443\nauth-user-pass\n"

// writeVPNConfigFile は、設定ファイルを一時ディレクトリに置き、そのパスを返す。
func writeVPNConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "provider.conf")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// secretNamed は、送るシークレットのうち name のものを返す。
func secretNamed(fields []vpnSecretField, name string) (string, bool) {
	for _, field := range fields {
		if field.name == name {
			return string(field.value), true
		}
	}
	return "", false
}

// 作成では、設定ファイルを読み、remote のサーバーを設定に、設定ファイルをシークレットにする。
func TestAddingAnOpenVPNProfileReadsTheConfigFile(t *testing.T) {
	path := writeVPNConfigFile(t, testOpenVPNConfig)
	p := profilePrompter(t, "openvpn\n\n"+path+"\nfixture\n", "fixture-password")

	input, err := readVPNProfile(p, "provider", nil)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	settings := input.profile.OpenVPN
	if settings == nil || strings.Join(settings.Servers, ",") != "vpn.example.jp,backup.example.jp" || settings.Username != "fixture" {
		t.Fatalf("openvpn = %+v", settings)
	}
	if config, sent := secretNamed(input.secrets, vpn.SecretKeyOpenVPNConfig); !sent || config != testOpenVPNConfig {
		t.Fatalf("設定ファイルを送っていない: %+v", input.secrets)
	}
	if password, sent := secretNamed(input.secrets, vpn.SecretKeyOpenVPNPassword); !sent || password != "fixture-password" {
		t.Fatalf("パスワードを送っていない: %+v", input.secrets)
	}
}

// 証明書だけで認証する設定ファイルでは、ユーザー名を空にでき、パスワードは聞かない。
func TestAnOpenVPNProfileWithoutAUsernameNeedsNoPassword(t *testing.T) {
	path := writeVPNConfigFile(t, "client\nremote vpn.example.jp\n")
	p := profilePrompter(t, "openvpn\n\n"+path+"\n\n")

	input, err := readVPNProfile(p, "provider", nil)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if _, sent := secretNamed(input.secrets, vpn.SecretKeyOpenVPNPassword); sent || input.profile.OpenVPN.Username != "" {
		t.Fatalf("profile = %+v, secrets = %+v", input.profile.OpenVPN, input.secrets)
	}
}

// 使えない指示を含む設定ファイルは、engine へ送る前に、行と指示を添えて断る。
func TestAnOpenVPNConfigWithARefusedDirectiveIsNotSent(t *testing.T) {
	path := writeVPNConfigFile(t, "client\nremote vpn.example.jp\nup /etc/openvpn/up.sh\n")
	p := profilePrompter(t, "openvpn\n\n"+path+"\n\n")

	_, err := readVPNProfile(p, "provider", nil)

	var input *vpnInputError
	if !errors.As(err, &input) || !strings.Contains(input.sentence, `Line 3: "up"`) {
		t.Fatalf("readVPNProfile = %v", err)
	}
}

// 見つからないファイルは、そう言って断る。
func TestAMissingOpenVPNConfigFileIsNamed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.ovpn")
	p := profilePrompter(t, "openvpn\n\n"+missing+"\n")

	_, err := readVPNProfile(p, "provider", nil)

	var input *vpnInputError
	if !errors.As(err, &input) || !strings.Contains(input.sentence, "The file does not exist.") {
		t.Fatalf("readVPNProfile = %v", err)
	}
}

// 編集では、設定ファイルのパスを空欄にすると、保存済みの設定ファイルとサーバーのまま残す。
func TestEditingAnOpenVPNProfileKeepsTheStoredFile(t *testing.T) {
	p := profilePrompter(t, "\n\n\n\n", "")
	p.editing = true
	saved := application.VPNProfile{
		Name: "provider", Backend: vpn.OpenVPN,
		OpenVPN: &application.OpenVPNProfile{Servers: []string{"vpn.example.jp"}, Username: "fixture"},
	}

	input, err := readVPNProfile(p, "provider", &saved)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if strings.Join(input.profile.OpenVPN.Servers, ",") != "vpn.example.jp" || input.profile.OpenVPN.Username != "fixture" {
		t.Fatalf("openvpn = %+v", input.profile.OpenVPN)
	}
	if len(input.secrets) != 0 {
		t.Fatalf("空欄のシークレットを送ろうとした: %+v", input.secrets)
	}
}
