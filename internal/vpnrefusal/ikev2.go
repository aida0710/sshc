package vpnrefusal

import "sshc/internal/vpn"

// ikev2SessionReasons は、IKEv2/IPsec の経路を用意できなかった理由の言い方である。
var ikev2SessionReasons = map[vpn.FailureReason]string{
	vpn.FailureIKEAuthentication:   "IKEv2の認証に失敗しました。ユーザー名、パスワード、事前共有鍵、IDを確認してください。",
	vpn.FailureIKEServerUnverified: "VPNサーバーの証明書を検証できませんでした。サーバーのIDとCAの証明書を確認してください。",
	vpn.FailureIKEProposalMismatch: "暗号スイートのネゴシエーションに失敗しました。IKEとESPの暗号スイートを確認してください。",
	vpn.FailureIKENoResponse: "VPNサーバーから応答がありません。サーバーの指定と、UDPの500番と4500番に届くかを" +
		"確認してください。",
	vpn.FailureXFRMInterface: "IPsecのXFRMインターフェースを作成できませんでした。DockerのLinuxカーネルが" +
		"対応していない可能性があります。",
}

// init は、IKEv2/IPsec に固有の理由を、共通の言い方の表に加える。文は Sentence が
// まとめて組み立てる。
func init() {
	for reason, sentence := range ikev2SessionReasons {
		sessionReasons[reason] = sentence
	}
}
