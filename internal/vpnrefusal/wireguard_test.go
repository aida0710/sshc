package vpnrefusal

import (
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// WireGuard で増えた理由の語も、どれも言い方を持つ。
func TestEveryWireGuardReasonHasASentence(t *testing.T) {
	for _, reason := range []vpn.Reason{
		vpn.ReasonUnknownDirective, vpn.ReasonMisplacedDirective, vpn.ReasonMissingDirective, vpn.ReasonDuplicate,
		vpn.ReasonNoEndpoint, vpn.ReasonNotInAllowedIPs, vpn.ReasonKeepaliveTooLong, vpn.ReasonMTUOutOfRange,
		vpn.ReasonConfigMismatch,
	} {
		if _, known := fieldReasons[reason]; !known {
			t.Errorf("%s has no field sentence", reason)
		}
	}
	if _, known := targetReasons[vpn.FailureTargetNotAllowed]; !known {
		t.Error("target_not_allowed has no target sentence")
	}
}

// testKey は、設定ファイルに書く鍵である。44 字の base64 で、値そのものに意味は無い。
const testKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="

// 設定ファイルの中の誤りは、行と項目を添えて言う。足りない項目は、どの項目かを言う。
func TestAWireGuardConfigRefusalNamesTheLineAndTheDirective(t *testing.T) {
	for _, test := range []struct {
		config string
		want   string
	}{
		{"[Interface]\nPrivateKey = " + testKey + "\nAddress = 10.0.0.2/32\nPostUp = iptables -A FORWARD\n",
			"secrets.wireguardConfig: 4行目の「PostUp」は、コマンドやプログラムを実行する指示のため使用できません。"},
		{"[Interface]\nAddress = 10.0.0.2/32\n", "secrets.wireguardConfig: 設定ファイルに「PrivateKey」がありません。"},
		{"[Interface]\nPrivateKey = " + testKey + "\nAddress = 10.0.0.2/32\n[Peer]\nAllowedIPs = 10.0.0.0/8\n",
			"secrets.wireguardConfig: 4行目の[Peer]に「PublicKey」がありません。"},
		{"[Interface]\nPrivateKey = " + testKey + "\nAddress = 10.0.0.2/32\nEndpoint = vpn.example.jp:51820\n",
			"secrets.wireguardConfig: 4行目の「Endpoint」は、この節には書けません。"},
	} {
		_, err := vpn.ParseWireGuardConfig([]byte(test.config))
		refusal, known := Of(err)
		if !known {
			t.Fatalf("Of(%v) is not a refusal", err)
		}
		if sentence := Sentence(refusal); sentence != test.want {
			t.Errorf("sentence = %q, want %q", sentence, test.want)
		}
	}
}

// AllowedIPs の外の接続先は、何に失敗したかを先に書き、設定ファイルの AllowedIPs を指す。
func TestATargetOutsideTheAllowedIPsSaysWhere(t *testing.T) {
	refusal := Refusal{Code: CodeTargetFailed, Reason: string(vpn.FailureTargetNotAllowed)}

	sentence := Sentence(refusal)

	if !strings.HasPrefix(sentence, "VPN経由で接続先に接続できませんでした。") || !strings.Contains(sentence, "AllowedIPs") {
		t.Fatalf("sentence = %q", sentence)
	}
	if !refusal.RequiresAction() {
		t.Fatal("AllowedIPs を直さない限り同じ理由で断られるのに、再接続を繰り返す")
	}
}
