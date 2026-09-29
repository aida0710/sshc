package vpn

import (
	"errors"
	"strings"
	"testing"
)

// testOpenVPNKeyLine は、テストの設定ファイルに入れる鍵の1行である。ログから伏せる
// ことを確かめる。
const testOpenVPNKeyLine = "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgfixturefixturefix"

// testOpenVPNConfig は、証明書と鍵をインラインで含む、クライアントの設定ファイルである。
var testOpenVPNConfig = strings.Join([]string{
	"client",
	"dev tun",
	"proto udp",
	"remote vpn.example.jp 1194",
	"remote backup.example.jp 1194",
	"nobind",
	"<key>",
	"-----BEGIN PRIVATE KEY-----",
	testOpenVPNKeyLine,
	"-----END PRIVATE KEY-----",
	"</key>",
	"",
}, "\n")

func validOpenVPNProfile() Profile {
	return Profile{
		Name:    "provider",
		Backend: OpenVPN,
		OpenVPN: &OpenVPNSettings{Servers: []string{"vpn.example.jp", "backup.example.jp"}},
	}
}

func validOpenVPNSecrets() Secrets {
	return Secrets{OpenVPN: &OpenVPNSecrets{Config: testOpenVPNConfig}}
}

// 設定ファイルは、そのままコンテナへ渡る。サーバーの並びは、コンテナが先に名前解決を
// 確かめるのに使う。
func TestTheOpenVPNConfigIsHandedToTheContainer(t *testing.T) {
	document := decodeAgentDocument(t, validOpenVPNProfile(), validOpenVPNSecrets())

	if document.OpenVPN == nil || document.OpenVPN.Config != testOpenVPNConfig {
		t.Fatalf("openvpn = %+v", document.OpenVPN)
	}
	if strings.Join(document.OpenVPN.Servers, ",") != "vpn.example.jp,backup.example.jp" {
		t.Fatalf("servers = %v", document.OpenVPN.Servers)
	}
	if document.OpenVPN.Username != "" || document.OpenVPN.Password != "" {
		t.Fatalf("ユーザー名の無いプロファイルで認証情報を渡した: %+v", document.OpenVPN)
	}
}

// ユーザー名があれば、パスワードと一緒に渡す。
func TestTheOpenVPNCredentialsAreHandedOverWithTheUsername(t *testing.T) {
	profile := validOpenVPNProfile()
	profile.OpenVPN.Username = "fixture"
	secrets := validOpenVPNSecrets()
	secrets.OpenVPN.Password = "fixture-password"

	document := decodeAgentDocument(t, profile, secrets)

	if document.OpenVPN.Username != "fixture" || document.OpenVPN.Password != "fixture-password" {
		t.Fatalf("openvpn = %+v", document.OpenVPN)
	}
}

// ユーザー名を消したプロファイルでは、Vault に残ったパスワードを渡さない。
func TestAStoredPasswordIsNotHandedOverWithoutAUsername(t *testing.T) {
	secrets := validOpenVPNSecrets()
	secrets.OpenVPN.Password = "left-behind"

	document := decodeAgentDocument(t, validOpenVPNProfile(), secrets)

	if document.OpenVPN.Password != "" {
		t.Fatalf("password = %q", document.OpenVPN.Password)
	}
}

// 設定ファイルとプロファイルが食い違うもの、足りないものは、コンテナへ渡す前に断る。
func TestAnOpenVPNProfileIsRefusedWhenItDisagreesWithItsConfig(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*Profile, *OpenVPNSecrets)
		field   string
		reason  Reason
		setting bool
	}{
		{"設定ファイルが無い", func(_ *Profile, secrets *OpenVPNSecrets) { secrets.Config = "" },
			"secrets.openvpnConfig", ReasonRequired, false},
		{"設定ファイルが長すぎる", func(_ *Profile, secrets *OpenVPNSecrets) {
			secrets.Config = strings.Repeat("#\n", MaxOpenVPNConfigLength)
		}, "secrets.openvpnConfig", ReasonTooLong, true},
		{"設定ファイルが断る指示を含む", func(_ *Profile, secrets *OpenVPNSecrets) {
			secrets.Config += "up /bin/true\n"
		}, "secrets.openvpnConfig", ReasonRunsCommand, false},
		{"サーバーが設定ファイルの remote と合わない", func(profile *Profile, _ *OpenVPNSecrets) {
			profile.OpenVPN.Servers = []string{"vpn.example.jp"}
		}, "openvpn.servers", ReasonConfigMismatch, true},
		{"設定ファイルが求めるユーザー名が無い", func(_ *Profile, secrets *OpenVPNSecrets) {
			secrets.Config += "auth-user-pass\n"
		}, "openvpn.username", ReasonRequiredByConfig, true},
		{"ユーザー名があるのにパスワードが無い", func(profile *Profile, _ *OpenVPNSecrets) {
			profile.OpenVPN.Username = "fixture"
		}, "secrets.openvpnPassword", ReasonRequired, false},
		{"パスワードが改行を含む", func(profile *Profile, secrets *OpenVPNSecrets) {
			profile.OpenVPN.Username = "fixture"
			secrets.Password = "first\nsecond"
		}, "secrets.openvpnPassword", ReasonFormat, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validOpenVPNProfile()
			secrets := validOpenVPNSecrets()
			test.change(&profile, secrets.OpenVPN)

			err := profile.ValidateSecrets(secrets)

			var failure *FieldError
			if !errors.As(err, &failure) || failure.Field != test.field || failure.Reason != test.reason {
				t.Fatalf("ValidateSecrets = %v, want %s %s", err, test.field, test.reason)
			}
			if kind := errors.Is(err, ErrSettings); kind != test.setting {
				t.Fatalf("ValidateSecrets = %v, ErrSettings = %v, want %v", err, kind, test.setting)
			}
		})
	}
}

// 設定の形は、設定ファイルを見ずに確かめられる範囲で確かめる。
func TestAnOpenVPNProfileNeedsItsServers(t *testing.T) {
	for _, test := range []struct {
		name    string
		servers []string
		reason  Reason
	}{
		{"サーバーが無い", nil, ReasonRequired},
		{"空白を含むサーバー", []string{"vpn example"}, ReasonFormat},
		{"多すぎる", make([]string, maxOpenVPNServers+1), ReasonTooMany},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validOpenVPNProfile()
			profile.OpenVPN.Servers = test.servers

			var failure *FieldError
			if err := profile.Validate(); !errors.As(err, &failure) || failure.Field != "openvpn.servers" ||
				failure.Reason != test.reason {
				t.Fatalf("Validate = %v, want %s", err, test.reason)
			}
		})
	}
}

// ログからは、パスワードと、設定ファイルの鍵の行を伏せる。鍵は1行ずつログに現れうる。
func TestShownLogsHideTheOpenVPNSecrets(t *testing.T) {
	secrets := validOpenVPNSecrets()
	secrets.OpenVPN.Password = "fixture-password"
	logs := "AUTH: fixture-password\nkey " + testOpenVPNKeyLine + "\n-----BEGIN PRIVATE KEY-----\nremote vpn.example.jp 1194"

	shown := redact(logs, secrets)

	for _, hidden := range []string{"fixture-password", testOpenVPNKeyLine} {
		if strings.Contains(shown, hidden) {
			t.Fatalf("%q が伏せられていない: %s", hidden, shown)
		}
	}
	// 区切りの行と、鍵でない指示は伏せない。伏せると、ログから何が起きたかが読めない。
	for _, kept := range []string{"-----BEGIN PRIVATE KEY-----", "remote vpn.example.jp 1194"} {
		if !strings.Contains(shown, kept) {
			t.Fatalf("%q まで伏せた: %s", kept, shown)
		}
	}
}

// Vault の記録のキーは保存形式の一部である。記録から戻すと同じ値になる。
func TestTheOpenVPNSecretsSurviveTheirStoredForm(t *testing.T) {
	stored := Secrets{OpenVPN: &OpenVPNSecrets{Config: testOpenVPNConfig, Password: "fixture-password"}}

	document, err := EncodeSecrets(stored)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{SecretKeyOpenVPNConfig, SecretKeyOpenVPNPassword} {
		if !strings.Contains(document, `"`+key+`"`) {
			t.Errorf("記録に %q が無い: %s", key, document)
		}
	}
	decoded, err := DecodeSecrets(document)
	if err != nil || decoded.OpenVPN == nil || *decoded.OpenVPN != *stored.OpenVPN {
		t.Fatalf("DecodeSecrets = %+v, %v", decoded, err)
	}
	if own := validOpenVPNProfile().OwnSecrets(decoded); own.OpenVPN == nil || own.WireGuard != nil {
		t.Fatalf("OwnSecrets = %+v", own)
	}
}

// OpenVPN は、サーバーが配る経路も DNS も入れない。interface のアドレスは /32 で付け、
// 接続先への経路は connect が1つずつ作る。
//
// OpenVPN が呼ぶ script はイメージに焼いてある。設定を置く tmpfs は noexec なので、
// そこへ書いたものは実行できない。
func TestOpenVPNNeverInstallsTheRoutesTheServerPushes(t *testing.T) {
	script, err := container.ReadFile("container/backend-openvpn.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	for _, wanted := range []string{
		"--route-noexec", "--ifconfig-noexec", "--pull-filter ignore redirect-gateway", "--pull-filter ignore route",
		"--pull-filter ignore dhcp-option", "--dev \"$interface\"", "--auth-nocache", "--auth-retry none",
	} {
		if !strings.Contains(body, wanted) {
			t.Errorf("OpenVPN の起動に %q が無い", wanted)
		}
	}
	// pull-filter は最初に合った規則を使う。設定ファイルの規則より先に置く。
	if strings.Index(body, "--pull-filter") > strings.Index(body, "--config") {
		t.Error("pull-filter が設定ファイルより後にある")
	}
	// interface などの sshc が決める指定は、設定ファイルより後に置く。後のものが使われる。
	if strings.Index(body, "--dev \"$interface\"") < strings.Index(body, "--config") {
		t.Error("dev が設定ファイルより前にある")
	}

	up, err := container.ReadFile("container/openvpn-up")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), `"$ifconfig_local/32"`) || strings.Contains(string(up), "ip route") {
		t.Fatalf("openvpn-up = %s", up)
	}
}

// パスワードは、引数にも環境変数にも置かない。tmpfs の上のファイルで渡す。
func TestTheOpenVPNPasswordNeverReachesTheCommandLine(t *testing.T) {
	script, err := container.ReadFile("container/backend-openvpn.sh")
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	if !strings.Contains(body, `jq -r '.openvpn.username, .openvpn.password' "$profile" >"$openvpn_credentials"`) {
		t.Fatal("パスワードをファイルへ直に書いていない")
	}
	if strings.Contains(body, "password=") || strings.Contains(body, "$password") {
		t.Fatal("パスワードをシェルの変数に置いている")
	}
}
