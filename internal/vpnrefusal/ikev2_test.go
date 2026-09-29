package vpnrefusal

import (
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// IKEv2/IPsec で増えた理由の語も、どれも言い方を持ち、何に失敗したかを先に書く。
func TestEveryIKEv2ReasonHasASentence(t *testing.T) {
	for _, reason := range []vpn.FailureReason{
		vpn.FailureIKEAuthentication, vpn.FailureIKEServerUnverified, vpn.FailureIKEProposalMismatch,
		vpn.FailureIKENoResponse, vpn.FailureXFRMInterface,
	} {
		sentence := Sentence(Refusal{Code: CodeSessionFailed, Reason: string(reason)})
		if !strings.HasPrefix(sentence, "VPNの接続に失敗しました。") || sentence == Sentence(Refusal{Code: CodeSessionFailed}) {
			t.Errorf("%s: sentence = %q", reason, sentence)
		}
	}
}

// サーバーの設定か、こちらの設定を直さない限り、IKEv2 の失敗は同じ理由で繰り返す。
// Terminal は再接続を繰り返さない。
func TestIKEv2RefusalsStopTheReconnect(t *testing.T) {
	for _, reason := range []vpn.FailureReason{vpn.FailureIKEAuthentication, vpn.FailureIKENoResponse} {
		if refusal := (Refusal{Code: CodeSessionFailed, Reason: string(reason)}); !refusal.RequiresAction() {
			t.Errorf("%s は再接続を繰り返す", reason)
		}
	}
}
