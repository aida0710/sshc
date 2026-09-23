package vpn

import (
	"encoding/json"
	"strings"
	"testing"
)

func validOpenConnectProfile() Profile {
	return Profile{
		Name:    "office",
		Backend: OpenConnect,
		Target:  Endpoint{Host: "10.9.9.1", Port: 22},
		OpenConnect: &OpenConnectSettings{
			Server:   "vpn.example.jp",
			Username: "fixture",
		},
	}
}

// 方式を書かなければ anyconnect になる。ocserv もこれで繋がる。
func TestAnOpenConnectProfileWithoutAProtocolSpeaksAnyConnect(t *testing.T) {
	profile := validOpenConnectProfile()

	document := decodeAgentDocument(t, profile, Secrets{OpenConnectPassword: "fixture-password"})

	if document.OpenConnect == nil || document.OpenConnect.Protocol != "anyconnect" {
		t.Fatalf("openconnect = %+v", document.OpenConnect)
	}
}

// 装置が配る既定経路もDNSも入れない。経路は接続先ひとつぶんだけにする。
func TestTheOpenConnectScriptOnlyBringsUpTheInterface(t *testing.T) {
	document := decodeAgentDocument(t, validOpenConnectProfile(), Secrets{OpenConnectPassword: "fixture-password"})

	script := document.OpenConnect.Script
	if !strings.Contains(script, "ip address add") || !strings.Contains(script, "ip link set dev") {
		t.Fatalf("script = %q", script)
	}
	for _, forbidden := range []string{"default", "resolv.conf", "CISCO_SPLIT_INC"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("script が %q に触れている: %q", forbidden, script)
		}
	}
}

// 経路を作れない指定は、コンテナへ届く前に断る。
func TestAnOpenConnectProfileIsRefusedWhenItCannotBecomeARoute(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*OpenConnectSettings)
	}{
		{"装置が空", func(settings *OpenConnectSettings) { settings.Server = "" }},
		{"装置に空白", func(settings *OpenConnectSettings) { settings.Server = "vpn example jp" }},
		{"利用者名が空", func(settings *OpenConnectSettings) { settings.Username = "" }},
		{"知らない方式", func(settings *OpenConnectSettings) { settings.Protocol = "openvpn" }},
		{"指紋の形が違う", func(settings *OpenConnectSettings) { settings.ServerCertificate = "abc123" }},
		{"指紋に空白", func(settings *OpenConnectSettings) { settings.ServerCertificate = "sha256:a b" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validOpenConnectProfile()
			settings := *profile.OpenConnect
			profile.OpenConnect = &settings
			test.change(profile.OpenConnect)

			if err := profile.Validate(); err == nil {
				t.Fatal("経路を作れない指定を受け取った")
			}
		})
	}
}

// パスワードが無ければ、繋ぎに行かない。
func TestOpenConnectNeedsItsPasswordBeforeConnecting(t *testing.T) {
	profile := validOpenConnectProfile()

	if err := profile.ValidateSecrets(Secrets{}); err == nil {
		t.Fatal("パスワード無しで繋ごうとした")
	}
	if err := profile.ValidateSecrets(Secrets{OpenConnectPassword: "fixture-password"}); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}

// 見せるログには、openconnect のパスワードも現れない。
func TestShownLogsHideTheOpenConnectPassword(t *testing.T) {
	secrets := Secrets{OpenConnectPassword: "fixture-password"}

	shown := redact("認証に失敗しました: fixture-password", secrets)

	if strings.Contains(shown, secrets.OpenConnectPassword) {
		t.Fatalf("redact = %q", shown)
	}
}

func decodeAgentDocument(t *testing.T, profile Profile, secrets Secrets) agentDocument {
	t.Helper()
	encoded, err := newAgentDocument(profile, secrets, 1000)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}
	var document agentDocument
	if err := json.Unmarshal([]byte(encoded), &document); err != nil {
		t.Fatal(err)
	}
	return document
}
