package vpn

import (
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
		Name:      "tohoku",
		Backend:   WireGuard,
		WireGuard: &WireGuardSettings{Servers: []string{"vpn.example.jp"}},
	}
}

// testWireGuardFields は、validProfile に合う設定ファイルを組み立てる項目である。
func testWireGuardFields() WireGuardFields {
	return WireGuardFields{Server: "vpn.example.jp:51820", PeerPublicKey: testPublicKey, Address: "10.9.9.2/32"}
}

// validSecrets は、validProfile に合う設定ファイルである。
func validSecrets() Secrets {
	return Secrets{WireGuard: &WireGuardSecrets{Config: testWireGuardFields().Config(testPrivateKey)}}
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
		{"知らないbackend", func(profile *Profile) { profile.Backend = "sstp" }, ErrBackend},
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
		{"サーバーが無い", func(profile *Profile) { profile.WireGuard.Servers = nil }, ErrSettings},
		{"サーバーに空白がある", func(profile *Profile) {
			profile.WireGuard.Servers = []string{"vpn example.jp"}
		}, ErrSettings},
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

func TestSecretsAreRefusedWhenTheBackendCannotUseThem(t *testing.T) {
	withConfig := func(fields WireGuardFields) Secrets {
		return Secrets{WireGuard: &WireGuardSecrets{Config: fields.Config(testPrivateKey)}}
	}
	shortPublicKey, missingPort, otherServer := testWireGuardFields(), testWireGuardFields(), testWireGuardFields()
	shortPublicKey.PeerPublicKey = "short"
	missingPort.Server = "vpn.example.jp"
	otherServer.Server = "other.example.jp:51820"
	withDNS := testWireGuardFields()
	withDNS.DNS = []string{"10.9.9.53"}
	for _, test := range []struct {
		name    string
		secrets Secrets
		field   string
		reason  Reason
	}{
		{"設定ファイルが無い", Secrets{}, "secrets.wireguardConfig", ReasonRequired},
		{"秘密鍵の形式が違う", Secrets{WireGuard: &WireGuardSecrets{
			Config: testWireGuardFields().Config("not-a-key"),
		}}, "secrets.wireguardConfig", ReasonFormat},
		{"相手の公開鍵が短い", withConfig(shortPublicKey), "secrets.wireguardConfig", ReasonFormat},
		{"サーバーのポートが無い", withConfig(missingPort), "secrets.wireguardConfig", ReasonFormat},
		{"設定ファイルのサーバーがプロファイルと違う", withConfig(otherServer), "wireguard.servers", ReasonConfigMismatch},
		{"設定ファイルの DNS がプロファイルと違う", withConfig(withDNS), "dns", ReasonConfigMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			var failure *FieldError
			if err := validProfile().ValidateSecrets(test.secrets); !errors.As(err, &failure) {
				t.Fatalf("ValidateSecrets = %v, want a refusal", err)
			}
			if failure.Field != test.field || failure.Reason != test.reason {
				t.Fatalf("failure = %+v, want %s %s", failure, test.field, test.reason)
			}
		})
	}
	if err := validProfile().ValidateSecrets(validSecrets()); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
	profile := validProfile()
	profile.DNS = withDNS.DNS
	if err := profile.ValidateSecrets(withConfig(withDNS)); err != nil {
		t.Fatalf("設定ファイルと同じ DNS のプロファイル: ValidateSecrets = %v", err)
	}
}

func TestAnInvalidProfileNeverReachesTheContainer(t *testing.T) {
	profile := validProfile()
	profile.DNS = []string{"dns.example.jp"}

	if _, err := newAgentDocument(profile, validSecrets(), testClock); !errors.Is(err, ErrSettings) {
		t.Fatalf("newAgentDocument = %v, want %v", err, ErrSettings)
	}
}

// 表示するログに、設定ファイルの秘密鍵が残らない。
func TestShownLogsHideThePrivateKey(t *testing.T) {
	logs := "wireguard-go: failed with key " + testPrivateKey + " again"

	shown := redactLogs(logs, validSecrets())

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
	document, err := EncodeSecrets(validSecrets())
	if err != nil {
		t.Fatalf("EncodeSecrets = %v", err)
	}
	if !strings.Contains(document, testPrivateKey) {
		t.Fatalf("記録に秘密鍵が入っていない: %q", document)
	}

	secrets, err := DecodeSecrets(document)
	if err != nil || secrets.WireGuard.Config != validSecrets().WireGuard.Config {
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
		{"サーバーが多すぎる", func(profile *Profile) {
			profile.WireGuard.Servers = make([]string, maxWireGuardPeers+1)
			for index := range profile.WireGuard.Servers {
				profile.WireGuard.Servers[index] = "vpn.example.jp"
			}
		}, "wireguard.servers", ReasonTooMany, maxWireGuardPeers},
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
		WireGuard:   &WireGuardSecrets{Config: "c"},
		L2TP:        &L2TPSecrets{Password: "p", PreSharedKey: "k"},
		OpenConnect: &OpenConnectSecrets{Password: "o", TOTPSecret: "t"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		SecretKeyWireGuardConfig, SecretKeyL2TPPassword, SecretKeyIPsecPSK, SecretKeyOpenConnectPassword,
		SecretKeyOpenConnectTOTPSecret,
	} {
		if !strings.Contains(document, `"`+key+`"`) {
			t.Errorf("記録に %q が無い: %s", key, document)
		}
	}
}

// v0.40.0 までの項目の形の記録から、秘密鍵を読める。設定ファイルの形の記録には無い。
func TestTheFieldsPrivateKeyIsReadFromAnOldRecord(t *testing.T) {
	key, err := DecodeWireGuardFieldsPrivateKey(`{"wireguardPrivateKey":"` + testPrivateKey + `"}`)
	if err != nil || key != testPrivateKey {
		t.Fatalf("DecodeWireGuardFieldsPrivateKey = %q, %v", key, err)
	}
	record, err := EncodeSecrets(validSecrets())
	if err != nil {
		t.Fatal(err)
	}
	if key, err := DecodeWireGuardFieldsPrivateKey(record); err != nil || key != "" {
		t.Fatalf("設定ファイルの形の記録: DecodeWireGuardFieldsPrivateKey = %q, %v", key, err)
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

// シークレットの長さは、API と同じ上限で確かめる。長すぎる値は、足りない
// シークレットではなく、使えない値として断る。
func TestAnOverlongSecretIsRefusedAsAnInvalidValue(t *testing.T) {
	profile := Profile{Name: "office", Backend: L2TPIPsec, L2TP: &L2TPSettings{Server: "vpn.example.jp", Username: "user"}}

	err := profile.ValidateSecrets(Secrets{L2TP: &L2TPSecrets{
		Password: strings.Repeat("p", maxSecretLength+1), PreSharedKey: "psk",
	}})

	var failure *FieldError
	if !errors.As(err, &failure) || !errors.Is(err, ErrSettings) || failure.Reason != ReasonTooLong ||
		failure.Field != "secrets."+SecretKeyL2TPPassword || failure.Limit != maxSecretLength {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}
