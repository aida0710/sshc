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
		WireGuard: &WireGuardSettings{
			Server:        Endpoint{Host: "vpn.example.jp", Port: 51820},
			PeerPublicKey: testPublicKey,
			Address:       "10.9.9.2/32",
		},
	}
}

func TestAProfileWithAServerAndKeysIsAccepted(t *testing.T) {
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
		{"DNSが名前", func(profile *Profile) {
			profile.DNS = []string{"dns.example.jp"}
		}, ErrSettings},
		{"DNSがループバック", func(profile *Profile) {
			profile.DNS = []string{"127.0.0.53"}
		}, ErrSettings},
		{"DNSが多すぎる", func(profile *Profile) {
			profile.DNS = []string{"10.9.9.1", "10.9.9.2", "10.9.9.3", "10.9.9.4"}
		}, ErrSettings},
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

// 起動したトンネルが運ぶのは、DNSサーバーへの通信だけである。
//
// 接続先は、接続に使われたものを connect が1つずつ足す。起動した時点で広く
// 開けると、VPNの向こうのネットワーク全体へ出られるトンネルになる。
func TestAStartedTunnelCarriesOnlyTheResolvers(t *testing.T) {
	profile := validProfile()
	profile.DNS = []string{"10.9.9.53", "10.9.9.54"}

	document, err := newAgentDocument(profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}}, testClock)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}

	var decoded agentDocument
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decoded.WireGuard.Configuration, "AllowedIPs = 10.9.9.53/32, 10.9.9.54/32\n") {
		t.Errorf("configuration = %q", decoded.WireGuard.Configuration)
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
		{"秘密鍵の形式が違う", Secrets{WireGuard: &WireGuardSecrets{PrivateKey: "not-a-key"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := profile.ValidateSecrets(test.secrets); err == nil {
				t.Fatal("ValidateSecrets accepted a secret the backend cannot use")
			}
		})
	}
	if err := profile.ValidateSecrets(Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}}); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}

// DNSサーバーの無い経路は、起動した時点で何も運ばない。
func TestATunnelWithoutResolversStartsCarryingNothing(t *testing.T) {
	profile := validProfile()

	document, err := newAgentDocument(profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}}, testClock)
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
	if strings.Contains(decoded.WireGuard.Configuration, "AllowedIPs") {
		t.Errorf("configuration = %q", decoded.WireGuard.Configuration)
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
	profile.DNS = []string{"dns.example.jp"}

	if _, err := newAgentDocument(profile, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}}, testClock); !errors.Is(err, ErrSettings) {
		t.Fatalf("newAgentDocument = %v, want %v", err, ErrSettings)
	}
}

// 表示するログに秘密鍵が残らない。
func TestShownLogsHideThePrivateKey(t *testing.T) {
	logs := "wireguard-go: failed with key " + testPrivateKey + " again"

	shown := redact(logs, Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}})

	if strings.Contains(shown, testPrivateKey) {
		t.Fatalf("redact kept the key: %q", shown)
	}
	if !strings.Contains(shown, "[REDACTED]") {
		t.Fatalf("redact = %q", shown)
	}
}

// 同じ機械の別の利用者や、同じ利用者の別の workspace のコンテナを、名前だけで掴まない。
func TestContainerNamesSeparateProfilesUsersAndWorkspaces(t *testing.T) {
	directory := t.TempDir()
	mine := New(directory, 1000, nil)
	names := map[string]string{
		"同じ設定":        mine.containerName("tohoku"),
		"別の利用者":       New(directory, 1001, nil).containerName("tohoku"),
		"別のプロファイル":    mine.containerName("office"),
		"別のworkspace": New(t.TempDir(), 1000, nil).containerName("tohoku"),
	}
	seen := map[string]string{}
	for label, name := range names {
		if previous, taken := seen[name]; taken {
			t.Fatalf("%s と %s が同じコンテナ名 %q になった", previous, label, name)
		}
		seen[name] = label
	}
	if New(directory, 1000, nil).containerName("tohoku") != names["同じ設定"] {
		t.Fatal("同じ workspace で起動し直した engine が、前回のコンテナを見つけられない")
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
	document, err := EncodeSecrets(Secrets{WireGuard: &WireGuardSecrets{PrivateKey: testPrivateKey}})
	if err != nil {
		t.Fatalf("EncodeSecrets = %v", err)
	}
	if !strings.Contains(document, testPrivateKey) {
		t.Fatalf("記録に秘密鍵が入っていない: %q", document)
	}

	secrets, err := DecodeSecrets(document)
	if err != nil || secrets.WireGuard.PrivateKey != testPrivateKey {
		t.Fatalf("DecodeSecrets = %+v, %v", secrets, err)
	}
	if _, err := DecodeSecrets("{"); !errors.Is(err, ErrSecrets) {
		t.Fatalf("DecodeSecrets(壊れた記録) = %v, want ErrSecrets", err)
	}
}

// 選んだ backend と違う backend の節は、経路の設定として受け取らない。
func TestAProfileCarryingAnotherBackendsSettingsIsRefused(t *testing.T) {
	profile := validProfile()
	profile.L2TP = &L2TPSettings{Server: "vpn.example.jp", Username: "user"}

	err := profile.Validate()

	var failure *FieldError
	if !errors.As(err, &failure) || failure.Field != "l2tp" || failure.Reason != ReasonUnexpected {
		t.Fatalf("Validate = %v", err)
	}
}

// 断る理由は、項目と理由の語で返る。画面と CLI はそれを翻訳して見せる。
func TestARefusalNamesTheFieldAndTheReason(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Profile)
		field  string
		reason Reason
		limit  int
	}{
		{"DNSが多すぎる", func(profile *Profile) {
			profile.DNS = []string{"10.9.9.1", "10.9.9.2", "10.9.9.3", "10.9.9.4"}
		}, "dns", ReasonTooMany, maxResolvers},
		{"公開鍵が短い", func(profile *Profile) { profile.WireGuard.PeerPublicKey = "short" }, "wireguard.peerPublicKey", ReasonFormat, 0},
		{"名前が長すぎる", func(profile *Profile) { profile.Name = strings.Repeat("a", maxProfileNameLength+1) }, "name", ReasonTooLong, maxProfileNameLength},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validProfile()
			settings := *profile.WireGuard
			profile.WireGuard = &settings
			test.change(&profile)

			var failure *FieldError
			if err := profile.Validate(); !errors.As(err, &failure) {
				t.Fatalf("Validate = %v", err)
			}
			if failure.Field != test.field || failure.Reason != test.reason || failure.Limit != test.limit {
				t.Fatalf("failure = %+v", failure)
			}
		})
	}
}

// DNS を使わない書き方が nil でも空の並びでも、同じ経路として扱う。
func TestAnEmptyResolverListIsTheSameRouteAsNone(t *testing.T) {
	withNil := validProfile()
	withEmpty := validProfile()
	withEmpty.DNS = []string{}

	if !withNil.sameRouteAs(withEmpty) {
		t.Fatal("DNS の書き方の違いだけで作り直す")
	}
}

// Vault の記録のキーは保存形式の一部である。変えるなら Vault の移行を足す。
func TestTheStoredSecretsKeepTheirKeys(t *testing.T) {
	document, err := EncodeSecrets(Secrets{
		L2TP:        &L2TPSecrets{Password: "p", PreSharedKey: "k"},
		OpenConnect: &OpenConnectSecrets{Password: "o", TOTPSecret: "t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{SecretKeyL2TPPassword, SecretKeyIPsecPSK, SecretKeyOpenConnectPassword, SecretKeyOpenConnectTOTPSecret} {
		if !strings.Contains(document, `"`+key+`"`) {
			t.Errorf("記録に %q が無い: %s", key, document)
		}
	}
}

// どの backend の手順も、agent.sh が呼ぶ関数をすべて持つ。
//
// 足りない関数があると、その backend だけがコンテナの中で「command not found」で
// 終わる。イメージを作らずに見つけられるのはここだけである。
func TestEveryBackendScriptDefinesTheAgentHooks(t *testing.T) {
	for name := range backends {
		script, err := container.ReadFile("container/backend-" + string(name) + ".sh")
		if err != nil {
			t.Fatalf("%s の手順が無い: %v", name, err)
		}
		for _, hook := range []string{"backend_read", "backend_up", "backend_ready", "backend_allow", "backend_alive", "backend_down"} {
			if !strings.Contains(string(script), hook+"()") {
				t.Errorf("%s の手順に %s が無い", name, hook)
			}
		}
	}
}

// コンテナは tini を PID 1 にして起こす。sh が PID 1 だと停止の合図を無視する。
func TestContainersStartWithAnInitProcess(t *testing.T) {
	arguments := runArguments(containerRun{
		name: "sshc-vpn-lab", image: "sshc-vpn:test", profile: validProfile(), owner: 1000,
		workspace: "000000000000", routeDirectory: "/tmp/lab", backend: backends[WireGuard],
	})

	if !strings.Contains(strings.Join(arguments, " "), " --init ") {
		t.Fatalf("arguments = %v", arguments)
	}
}
