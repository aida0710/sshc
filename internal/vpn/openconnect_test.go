package vpn

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// testClock は、二段目のコードを固定するための時刻である。
var testClock = time.Date(2026, 9, 23, 14, 0, 0, 0, time.UTC)

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

	document := decodeAgentDocument(t, profile, Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}})

	if document.OpenConnect == nil || document.OpenConnect.Protocol != "anyconnect" {
		t.Fatalf("openconnect = %+v", document.OpenConnect)
	}
}

// 装置が配る既定経路もDNSも入れない。経路は接続先ひとつぶんだけにする。
//
// openconnect が呼ぶ script はイメージに焼いてある。設定を置く tmpfs は noexec
// なので、そこへ書いたものは実行できない。
func TestTheOpenConnectScriptOnlyBringsUpTheInterface(t *testing.T) {
	script, err := container.ReadFile("container/vpnc-script")
	if err != nil {
		t.Fatal(err)
	}

	body := string(script)
	if !strings.Contains(body, "ip address add") || !strings.Contains(body, "ip link set dev") {
		t.Fatalf("script = %q", body)
	}
	for _, forbidden := range []string{"ip route", "resolv.conf", "CISCO_SPLIT_INC"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("script が %q に触れている: %q", forbidden, body)
		}
	}
}

// 二段目を聞いてくる装置には、決めた語をそのまま送る。
//
// openconnect は、パスワードの次の質問にも標準入力の次の行を使う。装置ごとに
// 違うフォームの名前を当てにしない。
func TestTheApprovalWordIsSentAsTheSecondAnswer(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorApprove
	profile.OpenConnect.ApprovalWord = "push"

	document := decodeAgentDocument(t, profile, Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}})

	if document.OpenConnect.SecondFactor != "push" {
		t.Fatalf("secondFactor = %q", document.OpenConnect.SecondFactor)
	}
}

// 何も聞かずに通知だけ出す装置へは、何も送らない。
//
// パスワードを受け取った時点で装置が Duo を鳴らし、承認されるまで応答を返さない
// 構成がある。ここで空の行を送ると、二段目を聞く装置に「空の答え」を渡すことに
// なり、認証が失敗した理由が分からなくなる。
func TestNothingIsSentWhenTheDeviceOnlyWaitsForApproval(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorApprove

	document := decodeAgentDocument(t, profile, Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}})

	if document.OpenConnect.SecondFactor != "" {
		t.Fatalf("secondFactor = %q", document.OpenConnect.SecondFactor)
	}
}

// 承認を待つことは、コンテナのログにも書く。
//
// 何も聞いてこない装置では、承認するまで出力が止まる。理由が書いていないと、
// 止まったのか待っているのかを利用者が区別できない。
func TestTheContainerIsToldThatItWaitsForApproval(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorApprove

	document := decodeAgentDocument(t, profile, Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}})

	if !document.OpenConnect.WaitsForApproval {
		t.Fatal("承認を待つことがコンテナへ伝わっていない")
	}
}

// 承認を待つ経路は、普通の経路より長く待つ。
//
// 通知に気づいて、電話を開いて、承認するまでを見込む。同じ長さで打ち切ると、
// 押す前に畳んでしまう。
func TestAnApprovedRouteIsGivenLongerToComeUp(t *testing.T) {
	profile := validOpenConnectProfile()

	plain := relayDeadline(profile)
	profile.OpenConnect.SecondFactor = SecondFactorApprove
	approved := relayDeadline(profile)

	if approved <= plain {
		t.Fatalf("承認を待つ経路の猶予 = %v、普通の経路 = %v", approved, plain)
	}
}

// TOTP の設定は、渡す直前の時刻でコードを作る。
func TestTheTOTPCodeIsMadeForTheMomentItIsHandedOver(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorTOTP
	secrets := Secrets{OpenConnect: &OpenConnectSecrets{
		Password: "fixture-password",
		// RFC 6238 の試験鍵（"12345678901234567890" の base32）。
		TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	}}

	document := decodeAgentDocument(t, profile, secrets)

	code := document.OpenConnect.SecondFactor
	if len(code) != 6 {
		t.Fatalf("secondFactor = %q", code)
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			t.Fatalf("secondFactor = %q", code)
		}
	}
	// 30 秒後は別のコードになる。固定値を送っていないことを確かめる。
	later, err := newAgentDocument(profile, secrets, 1000, testClock.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(later, code) {
		t.Fatalf("30秒後も同じコードだった: %q", code)
	}
}

// TOTP を選んだのに種が無ければ、繋ぎに行かない。
func TestTOTPAsTheSecondFactorNeedsItsSeed(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorTOTP

	if err := profile.ValidateSecrets(Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}}); err == nil {
		t.Fatal("種が無いまま繋ごうとした")
	}
}

// 二段目が無い装置へは、空の行を送らない。
//
// 空行を送ると、二段目を聞く装置に「空の答え」を渡すことになり、失敗の理由が
// 分からなくなる。
func TestNoSecondAnswerIsSentWhenTheDeviceAsksOnlyOnce(t *testing.T) {
	document := decodeAgentDocument(t, validOpenConnectProfile(), Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}})

	if document.OpenConnect.SecondFactor != "" {
		t.Fatalf("secondFactor = %q", document.OpenConnect.SecondFactor)
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
		{"指紋がsha1", func(settings *OpenConnectSettings) { settings.ServerCertificate = "sha1:abc123" }},
		{"指紋に空白", func(settings *OpenConnectSettings) { settings.ServerCertificate = "sha256:a b" }},
		{"知らない二段目", func(settings *OpenConnectSettings) { settings.SecondFactor = "sms-maybe" }},
		{"合図の語に空白", func(settings *OpenConnectSettings) {
			settings.SecondFactor = SecondFactorApprove
			settings.ApprovalWord = "push now"
		}},
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

// openconnect が受け取る2つの書き方は、どちらも通す。
func TestBothCertificateFingerprintFormsAreAccepted(t *testing.T) {
	for _, fingerprint := range []string{
		"sha256:1b229ed774a5dc31e441004e148c182bc9cdfcfc7366718f0e6a122ab0e659d5",
		"pin-sha256:kXHy0R7iOBX071y8voe47G1faCRMpykpg/0+8M7lX1M=",
	} {
		profile := validOpenConnectProfile()
		settings := *profile.OpenConnect
		profile.OpenConnect = &settings
		profile.OpenConnect.ServerCertificate = fingerprint

		if err := profile.Validate(); err != nil {
			t.Fatalf("Validate(%q) = %v", fingerprint, err)
		}
	}
}

// パスワードが無ければ、繋ぎに行かない。
func TestOpenConnectNeedsItsPasswordBeforeConnecting(t *testing.T) {
	profile := validOpenConnectProfile()

	if err := profile.ValidateSecrets(Secrets{}); err == nil {
		t.Fatal("パスワード無しで繋ごうとした")
	}
	if err := profile.ValidateSecrets(Secrets{OpenConnect: &OpenConnectSecrets{Password: "fixture-password"}}); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}

// 見せるログには、openconnect のパスワードも二段目の種も現れない。
func TestShownLogsHideTheOpenConnectPassword(t *testing.T) {
	secrets := Secrets{OpenConnect: &OpenConnectSecrets{
		Password:   "fixture-password",
		TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	}}

	shown := redact("認証に失敗しました: fixture-password / GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", secrets)

	for _, forbidden := range []string{secrets.OpenConnect.Password, secrets.OpenConnect.TOTPSecret} {
		if strings.Contains(shown, forbidden) {
			t.Fatalf("redact = %q", shown)
		}
	}
}

func decodeAgentDocument(t *testing.T, profile Profile, secrets Secrets) agentDocument {
	t.Helper()
	encoded, err := newAgentDocument(profile, secrets, 1000, testClock)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}
	var document agentDocument
	if err := json.Unmarshal([]byte(encoded), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

// コンテナは、engine が待つのをやめるより先に諦める。
//
// 先に engine が打ち切ると、利用者が受け取るのは「成立しませんでした」だけで、
// どの段階で何を待っていたのかがログに残らない。
func TestTheContainerGivesUpBeforeTheEngineStopsWaiting(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*Profile)
	}{
		{"普通の経路", func(*Profile) {}},
		{"承認を待つ経路", func(profile *Profile) {
			profile.OpenConnect.SecondFactor = SecondFactorApprove
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := validOpenConnectProfile()
			settings := *profile.OpenConnect
			profile.OpenConnect = &settings
			test.prepare(&profile)

			attempt := time.Duration(connectAttemptSeconds(profile)) * time.Second

			if attempt >= relayDeadline(profile) {
				t.Fatalf("コンテナの上限 %v が engine の上限 %v より短くない", attempt, relayDeadline(profile))
			}
			if attempt <= 0 {
				t.Fatalf("コンテナの上限 = %v", attempt)
			}
		})
	}
}

// 窓の終わり間際には、次の窓まで待ってからコードを作る。
func TestTheTOTPCodeIsNotMadeAtTheEndOfItsWindow(t *testing.T) {
	profile := validOpenConnectProfile()
	profile.OpenConnect.SecondFactor = SecondFactorTOTP
	secrets := Secrets{OpenConnect: &OpenConnectSecrets{
		Password: "fixture-password", TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	}}
	windowStart := time.Unix(1_800_000_000-1_800_000_000%30, 0)

	if wait := secondFactorWait(profile, secrets, windowStart.Add(10*time.Second)); wait != 0 {
		t.Fatalf("窓の中ほどで %v 待った", wait)
	}
	if wait := secondFactorWait(profile, secrets, windowStart.Add(28*time.Second)); wait != 2*time.Second {
		t.Fatalf("窓の終わり間際の待ち = %v", wait)
	}
	profile.OpenConnect.SecondFactor = ""
	if wait := secondFactorWait(profile, secrets, windowStart.Add(28*time.Second)); wait != 0 {
		t.Fatalf("TOTP を使わない経路で %v 待った", wait)
	}
}
