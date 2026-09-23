package vpn

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// 44字のbase64。値そのものに意味は無い。
const (
	testPrivateKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="
	testPublicKey  = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA="
)

func validProfile() Profile {
	return Profile{
		Name:    "tohoku",
		Backend: WireGuard,
		Target:  Endpoint{Host: "10.9.9.1", Port: 22},
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: "vpn.example.jp", Port: 51820},
			PeerPublicKey: testPublicKey,
			Address:       "10.9.9.2/32",
		},
	}
}

func TestAProfileWithAnAddressableTargetAndKeysIsAccepted(t *testing.T) {
	if err := validProfile().Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
}

func TestAProfileIsRefusedWhenTheRouteCouldNotBeBuiltFromIt(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Profile)
		want   error
	}{
		{"名前が空", func(profile *Profile) { profile.Name = "" }, ErrProfileName},
		{"名前にパス区切り", func(profile *Profile) { profile.Name = "../escape" }, ErrProfileName},
		{"知らないbackend", func(profile *Profile) { profile.Backend = "openvpn" }, ErrBackend},
		{"接続先が名前なのにDNSが無い", func(profile *Profile) { profile.Target.Host = "host.example.jp" }, ErrTarget},
		{"接続先の名前に空白", func(profile *Profile) {
			profile.Target.Host = "host example.jp"
			profile.DNS = []string{"10.9.9.53"}
		}, ErrTarget},
		{"DNSが名前", func(profile *Profile) {
			profile.DNS = []string{"dns.example.jp"}
		}, ErrTarget},
		{"DNSがループバック", func(profile *Profile) {
			profile.DNS = []string{"127.0.0.53"}
		}, ErrTarget},
		{"DNSが多すぎる", func(profile *Profile) {
			profile.DNS = []string{"10.9.9.1", "10.9.9.2", "10.9.9.3", "10.9.9.4"}
		}, ErrTarget},
		{"接続先がIPv6", func(profile *Profile) { profile.Target.Host = "2001:db8::1" }, ErrTarget},
		{"接続先がループバック", func(profile *Profile) { profile.Target.Host = "127.0.0.1" }, ErrTarget},
		{"ポートが範囲外", func(profile *Profile) { profile.Target.Port = 70000 }, ErrTarget},
		{"wireguardの設定が無い", func(profile *Profile) { profile.WireGuard = nil }, ErrSettings},
		{"相手の公開鍵が短い", func(profile *Profile) { profile.WireGuard.PeerPublicKey = "short" }, ErrSettings},
		{"公開鍵に改行", func(profile *Profile) {
			profile.WireGuard.PeerPublicKey = testPublicKey[:43] + "\n"
		}, ErrSettings},
		{"トンネル側アドレスがCIDRでない", func(profile *Profile) { profile.WireGuard.Address = "10.9.9.2" }, ErrSettings},
		{"サーバーのポートが無い", func(profile *Profile) { profile.WireGuard.Server.Port = 0 }, ErrSettings},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validProfile()
			settings := *profile.WireGuard
			profile.WireGuard = &settings
			test.change(&profile)

			err := profile.Validate()

			if !errors.Is(err, test.want) {
				t.Fatalf("Validate = %v, want %v", err, test.want)
			}
		})
	}
}

// 名前の接続先は、VPNの中のDNSを添えたときだけ受け取る。
func TestANamedTargetIsAcceptedOnceTheVPNHasItsOwnDNS(t *testing.T) {
	profile := validProfile()
	profile.Target.Host = "lab.example.jp"
	profile.DNS = []string{"10.9.9.53"}

	if err := profile.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
}

// 名前を引く前のトンネルが運ぶのは、DNSサーバーへの通信だけである。
//
// 接続先のアドレスはまだ分からない。ここで広く開けると、名前が引けなかった
// あとも余計な相手へ出られるトンネルが残る。
func TestATunnelForANamedTargetCarriesOnlyTheResolversUntilTheNameIsResolved(t *testing.T) {
	profile := validProfile()
	profile.Target.Host = "lab.example.jp"
	profile.DNS = []string{"10.9.9.53", "10.9.9.54"}

	document, err := newAgentDocument(profile, Secrets{WireGuardPrivateKey: testPrivateKey}, 1000)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}

	var decoded agentDocument
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decoded.WireGuard.Configuration, "AllowedIPs = 10.9.9.53/32, 10.9.9.54/32") {
		t.Errorf("configuration = %q", decoded.WireGuard.Configuration)
	}
	if strings.Contains(decoded.WireGuard.Configuration, "lab.example.jp") {
		t.Errorf("引けていない名前がトンネルの設定に入った: %q", decoded.WireGuard.Configuration)
	}
	if strings.Join(decoded.DNS, ",") != "10.9.9.53,10.9.9.54" {
		t.Errorf("dns = %v", decoded.DNS)
	}
}

func TestSecretsAreRefusedWhenTheBackendCannotUseThem(t *testing.T) {
	profile := validProfile()
	for _, test := range []struct {
		name    string
		secrets Secrets
	}{
		{"秘密鍵が無い", Secrets{}},
		{"秘密鍵の形式が違う", Secrets{WireGuardPrivateKey: "not-a-key"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := profile.ValidateSecrets(test.secrets); err == nil {
				t.Fatal("ValidateSecrets accepted a secret the backend cannot use")
			}
		})
	}
	if err := profile.ValidateSecrets(Secrets{WireGuardPrivateKey: testPrivateKey}); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}

// トンネルが運ぶのは、そのプロファイルの接続先への通信だけである。
func TestTheTunnelCarriesOnlyTheConfiguredTarget(t *testing.T) {
	profile := validProfile()

	document, err := newAgentDocument(profile, Secrets{WireGuardPrivateKey: testPrivateKey}, 1000)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}

	var decoded agentDocument
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.WireGuard == nil {
		t.Fatal("the document carries no wireguard configuration")
	}
	if !strings.Contains(decoded.WireGuard.Configuration, "AllowedIPs = 10.9.9.1/32") {
		t.Errorf("configuration = %q", decoded.WireGuard.Configuration)
	}
	if strings.Contains(decoded.WireGuard.Configuration, "0.0.0.0/0") {
		t.Errorf("the tunnel was given the whole network: %q", decoded.WireGuard.Configuration)
	}
	if decoded.Target.Host != "10.9.9.1" || decoded.Target.Port != 22 || decoded.SocketOwner != 1000 {
		t.Errorf("document = %+v", decoded)
	}
	// wg setconf は Address を読まない。別に渡す。
	if strings.Contains(decoded.WireGuard.Configuration, "Address") {
		t.Errorf("configuration carries Address: %q", decoded.WireGuard.Configuration)
	}
	if decoded.WireGuard.Address != "10.9.9.2/32" {
		t.Errorf("address = %q", decoded.WireGuard.Address)
	}
}

func TestAnInvalidProfileNeverReachesTheContainer(t *testing.T) {
	profile := validProfile()
	profile.Target.Host = "example.jp"

	if _, err := newAgentDocument(profile, Secrets{WireGuardPrivateKey: testPrivateKey}, 1000); !errors.Is(err, ErrTarget) {
		t.Fatalf("newAgentDocument = %v, want %v", err, ErrTarget)
	}
}

// 表示するログに秘密鍵が残らない。
func TestShownLogsHideThePrivateKey(t *testing.T) {
	logs := "wireguard-go: failed with key " + testPrivateKey + " again"

	shown := redact(logs, Secrets{WireGuardPrivateKey: testPrivateKey})

	if strings.Contains(shown, testPrivateKey) {
		t.Fatalf("redact kept the key: %q", shown)
	}
	if !strings.Contains(shown, "[REDACTED]") {
		t.Fatalf("redact = %q", shown)
	}
}

// 同じ機械の別の利用者のコンテナを、名前だけで掴まない。
func TestContainerNamesSeparateProfilesAndUsers(t *testing.T) {
	if containerName("tohoku", 1000) == containerName("tohoku", 1001) {
		t.Fatal("two users share one container name")
	}
	if containerName("tohoku", 1000) == containerName("office", 1000) {
		t.Fatal("two profiles share one container name")
	}
}

// イメージの中身が変われば、そのイメージを指す名前も変わる。
func TestTheImageTagFollowsTheEmbeddedContents(t *testing.T) {
	tag, err := imageTag()
	if err != nil {
		t.Fatal(err)
	}
	again, err := imageTag()
	if err != nil {
		t.Fatal(err)
	}
	if tag != again {
		t.Fatalf("imageTag is not stable: %q then %q", tag, again)
	}
	if !strings.HasPrefix(tag, imageName+":") || len(tag) != len(imageName)+13 {
		t.Fatalf("imageTag = %q", tag)
	}
}

// Vault へ保存する記録は、読み書きで同じ値に戻る。
func TestSecretsSurviveTheirStoredForm(t *testing.T) {
	document, err := EncodeSecrets(Secrets{WireGuardPrivateKey: testPrivateKey})
	if err != nil {
		t.Fatalf("EncodeSecrets = %v", err)
	}
	if !strings.Contains(document, testPrivateKey) {
		t.Fatalf("記録に秘密鍵が入っていない: %q", document)
	}

	secrets, err := DecodeSecrets(document)
	if err != nil || secrets.WireGuardPrivateKey != testPrivateKey {
		t.Fatalf("DecodeSecrets = %+v, %v", secrets, err)
	}
	if _, err := DecodeSecrets("{"); !errors.Is(err, ErrSecrets) {
		t.Fatalf("DecodeSecrets(壊れた記録) = %v, want ErrSecrets", err)
	}
}
