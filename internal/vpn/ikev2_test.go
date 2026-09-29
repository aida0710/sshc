package vpn

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func ikev2EAPProfile() Profile {
	return Profile{
		Name:    "office",
		Backend: IKEv2,
		IKEv2: &IKEv2Settings{
			Server:         "vpn.example.jp",
			Authentication: IKEv2AuthenticationEAP,
			Identity:       "vpn-user",
		},
	}
}

func ikev2PSKProfile() Profile {
	return Profile{
		Name:    "branch",
		Backend: IKEv2,
		IKEv2: &IKEv2Settings{
			Server:         "vpn.example.jp",
			Authentication: IKEv2AuthenticationPSK,
			Identity:       "branch@example.jp",
		},
	}
}

func ikev2Secrets() Secrets {
	return Secrets{IKEv2: &IKEv2Secrets{Password: `p"a\ss`, PreSharedKey: "shared-secret"}}
}

func TestIKEv2ProfilesWithEitherAuthenticationAreAccepted(t *testing.T) {
	withAuthority := ikev2EAPProfile()
	withAuthority.IKEv2.CACertificate = newTestAuthority(t, "office CA").PEM
	for _, profile := range []Profile{ikev2EAPProfile(), ikev2PSKProfile(), withAuthority} {
		if err := profile.Validate(); err != nil {
			t.Fatalf("Validate(%+v) = %v", profile.IKEv2, err)
		}
	}
}

// 断る理由は、項目と理由の語で返す。画面と CLI はそれを項目の横に出す。
func TestIKEv2SettingsAreRefusedWithTheFieldAndTheReason(t *testing.T) {
	authority := newTestAuthority(t, "office CA")
	_, privateKey := authority.issueServerCertificate(t, "vpn.example.jp")
	for _, test := range []struct {
		name   string
		change func(*IKEv2Settings)
		field  string
		reason Reason
	}{
		{"サーバーが無い", func(settings *IKEv2Settings) { settings.Server = "" }, "ikev2.server", ReasonRequired},
		{"認証の方式が無い", func(settings *IKEv2Settings) { settings.Authentication = "" },
			"ikev2.authentication", ReasonRequired},
		{"知らない認証の方式", func(settings *IKEv2Settings) { settings.Authentication = "eap-tls" },
			"ikev2.authentication", ReasonUnsupported},
		{"ID が無い", func(settings *IKEv2Settings) { settings.Identity = "" }, "ikev2.identity", ReasonRequired},
		{"ID に引用符", func(settings *IKEv2Settings) { settings.Identity = `a"b` }, "ikev2.identity", ReasonFormat},
		{"サーバーの ID が %any", func(settings *IKEv2Settings) { settings.ServerIdentity = "%any" },
			"ikev2.serverIdentity", ReasonFormat},
		{"サーバーの ID に改行", func(settings *IKEv2Settings) { settings.ServerIdentity = "vpn\n" },
			"ikev2.serverIdentity", ReasonFormat},
		{"CA の証明書が PEM でない", func(settings *IKEv2Settings) { settings.CACertificate = "certificate" },
			"ikev2.caCertificate", ReasonFormat},
		{"CA の証明書の代わりに秘密鍵", func(settings *IKEv2Settings) { settings.CACertificate = privateKey },
			"ikev2.caCertificate", ReasonFormat},
		{"CA の証明書の前に文字", func(settings *IKEv2Settings) { settings.CACertificate = "ca:\n" + authority.PEM },
			"ikev2.caCertificate", ReasonFormat},
		{"CA の証明書の中身が証明書でない", func(settings *IKEv2Settings) {
			settings.CACertificate = "-----BEGIN CERTIFICATE-----\nMAMCAQE=\n-----END CERTIFICATE-----\n"
		}, "ikev2.caCertificate", ReasonFormat},
		{"CA の証明書が長すぎる", func(settings *IKEv2Settings) {
			settings.CACertificate = strings.Repeat(authority.PEM, maxCACertificateLength/len(authority.PEM)+1)
		}, "ikev2.caCertificate", ReasonTooLong},
		{"事前共有鍵なのに CA の証明書", func(settings *IKEv2Settings) {
			settings.Authentication = IKEv2AuthenticationPSK
			settings.CACertificate = authority.PEM
		}, "ikev2.caCertificate", ReasonUnexpected},
		{"ESP に空白", func(settings *IKEv2Settings) { settings.ESP = "aes256 sha256" }, "ikev2.esp", ReasonFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := ikev2EAPProfile()
			test.change(profile.IKEv2)

			err := profile.Validate()

			var failure *FieldError
			if !errors.As(err, &failure) || !errors.Is(err, ErrSettings) ||
				failure.Field != test.field || failure.Reason != test.reason {
				t.Fatalf("Validate = %v, want %s %s", err, test.field, test.reason)
			}
		})
	}
}

// 認証の方式が要るシークレットが無いまま接続しない。
func TestIKEv2SecretsFollowTheAuthentication(t *testing.T) {
	for _, test := range []struct {
		name    string
		profile Profile
		secrets Secrets
		field   string
	}{
		{"EAP にパスワードが無い", ikev2EAPProfile(), Secrets{IKEv2: &IKEv2Secrets{PreSharedKey: "psk"}},
			"secrets." + SecretKeyIKEv2Password},
		{"事前共有鍵が無い", ikev2PSKProfile(), Secrets{IKEv2: &IKEv2Secrets{Password: "password"}},
			"secrets." + SecretKeyIKEv2PSK},
		{"シークレットが無い", ikev2EAPProfile(), Secrets{}, "secrets." + SecretKeyIKEv2Password},
	} {
		t.Run(test.name, func(t *testing.T) {
			var failure *FieldError
			err := test.profile.ValidateSecrets(test.secrets)
			if !errors.As(err, &failure) || !errors.Is(err, ErrSecrets) || failure.Field != test.field {
				t.Fatalf("ValidateSecrets = %v, want %s", err, test.field)
			}
		})
	}
	for _, profile := range []Profile{ikev2EAPProfile(), ikev2PSKProfile()} {
		if err := profile.ValidateSecrets(ikev2Secrets()); err != nil {
			t.Fatalf("ValidateSecrets = %v", err)
		}
	}
}

// 表示するログには、パスワードも事前共有鍵も、その16進表記も残らない。
func TestShownLogsHideEveryIKEv2Secret(t *testing.T) {
	secrets := ikev2Secrets()
	var logs []string
	for _, value := range []string{secrets.IKEv2.Password, secrets.IKEv2.PreSharedKey} {
		encoded := hex.EncodeToString([]byte(value))
		logs = append(logs, "charon: "+value, "swanctl: 0x"+encoded, "swanctl: 0x"+strings.ToUpper(encoded))
	}

	shown := redact(strings.Join(logs, "\n"), secrets)

	for _, value := range []string{secrets.IKEv2.Password, secrets.IKEv2.PreSharedKey} {
		encoded := hex.EncodeToString([]byte(value))
		for _, forbidden := range []string{value, encoded, strings.ToUpper(encoded)} {
			if strings.Contains(shown, forbidden) {
				t.Fatalf("ログに %q が残った: %s", forbidden, shown)
			}
		}
	}
}

// 方式を切り替えたあとに、ほかの方式のシークレットを残さない。
func TestIKEv2KeepsOnlyItsOwnSecrets(t *testing.T) {
	all := Secrets{
		WireGuard: &WireGuardSecrets{Config: testWireGuardConfig},
		L2TP:      &L2TPSecrets{Password: "l2tp", PreSharedKey: "l2tp-psk"},
		IKEv2:     &IKEv2Secrets{Password: "ikev2"},
	}

	own := ikev2EAPProfile().OwnSecrets(all)

	if own.WireGuard != nil || own.L2TP != nil || own.OpenConnect != nil || own.IKEv2 == nil ||
		own.IKEv2.Password != "ikev2" {
		t.Fatalf("OwnSecrets = %+v", own)
	}
}

// シークレットは、Vault の記録の形を通っても変わらない。
func TestIKEv2SecretsSurviveTheVaultRecord(t *testing.T) {
	encoded, err := EncodeSecrets(ikev2Secrets())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, `"ikev2Password"`) || !strings.Contains(encoded, `"ikev2Psk"`) {
		t.Fatalf("記録のキーが違う: %s", encoded)
	}
	decoded, err := DecodeSecrets(encoded)
	if err != nil || decoded.IKEv2 == nil || *decoded.IKEv2 != *ikev2Secrets().IKEv2 {
		t.Fatalf("DecodeSecrets = %+v, %v", decoded, err)
	}
}

// IKEv2 はトンネルのデバイスを要らない。要らないものはコンテナへ渡さない。
func TestIKEv2ContainersGetNoDevice(t *testing.T) {
	arguments := runArguments(containerRun{
		name: "sshc-vpn-office", image: "sshc-vpn:test", profile: ikev2EAPProfile(), owner: 1000,
		workspace: "000000000000", routeDirectory: "/tmp/office", backend: backends[IKEv2],
	})

	joined := strings.Join(arguments, " ")
	if strings.Contains(joined, "--device") || !strings.Contains(joined, "--cap-add NET_ADMIN") {
		t.Fatalf("arguments = %v", arguments)
	}
}
