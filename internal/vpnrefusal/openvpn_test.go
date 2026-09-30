package vpnrefusal

import (
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// OpenVPN で増えた理由の語も、どれも言い方を持つ。
func TestEveryOpenVPNReasonHasASentence(t *testing.T) {
	for _, reason := range []vpn.Reason{
		vpn.ReasonRunsCommand, vpn.ReasonChangesRoutes, vpn.ReasonDecidedBySshc, vpn.ReasonFileReference,
		vpn.ReasonServerMode, vpn.ReasonUnsupportedInline, vpn.ReasonUnclosedInline, vpn.ReasonNotClient,
		vpn.ReasonNoRemote, vpn.ReasonConfigMismatch, vpn.ReasonRequiredByConfig,
	} {
		if _, known := fieldReasons[reason]; !known {
			t.Errorf("%s has no field sentence", reason)
		}
	}
	for _, reason := range []vpn.FailureReason{
		vpn.FailureOpenVPNAuthentication, vpn.FailureOpenVPNTLS, vpn.FailureOpenVPNNoResponse,
		vpn.FailureOpenVPNConfiguration, vpn.FailureOpenVPN,
	} {
		if _, known := routeReasons[reason]; !known {
			t.Errorf("%s has no route sentence", reason)
		}
	}
}

// 設定ファイルの中の誤りは、何行目のどの指示かを添えて言う。利用者が設定ファイルの
// どこを直せばよいかが分かる。
func TestARefusedDirectiveIsNamedWithItsLine(t *testing.T) {
	_, err := vpn.InspectOpenVPNConfig([]byte("client\nremote vpn.example.jp\nca /etc/openvpn/ca.crt\n"))

	refusal, known := Of(err)
	if !known || refusal.Code != CodeSecretsMissing || refusal.Line != 3 || refusal.Directive != "ca" {
		t.Fatalf("Of = %+v, %v", refusal, known)
	}
	sentence := Sentence(refusal)
	for _, wanted := range []string{"3行目の「ca」", "<ca>〜</ca>"} {
		if !strings.Contains(sentence, wanted) {
			t.Fatalf("sentence = %q, %q が無い", sentence, wanted)
		}
	}
}

// 指示を名指ししない誤りも、行を添えて言う。
func TestALineWithoutADirectiveIsStillNamed(t *testing.T) {
	sentence := Sentence(Refusal{
		Code: CodeSecretsMissing, Field: "secrets.openvpnConfig", Reason: string(vpn.ReasonTooLong), Limit: 254, Line: 7,
	})

	if sentence != "secrets.openvpnConfig：7行目：長すぎます（254文字まで）。" {
		t.Fatalf("sentence = %q", sentence)
	}
}

// 認証の失敗は、何に失敗したかを先に書く。
func TestAnOpenVPNAuthenticationFailureSaysWhatFailed(t *testing.T) {
	sentence := Sentence(Refusal{Code: CodeRouteFailed, Reason: string(vpn.FailureOpenVPNAuthentication)})

	if !strings.HasPrefix(sentence, "VPNの接続に失敗しました。VPNサーバーが認証を拒否しました。") {
		t.Fatalf("sentence = %q", sentence)
	}
}
