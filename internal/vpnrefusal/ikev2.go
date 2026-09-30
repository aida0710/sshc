package vpnrefusal

import "sshc/internal/vpn"

// ikev2RouteReasons は、IKEv2/IPsec の経路を用意できなかった理由の言い方である。
var ikev2RouteReasons = map[vpn.FailureReason]string{
	vpn.FailureIKEAuthentication:   "IKEv2の認証に失敗しました。ユーザー名、パスワード、事前共有鍵、IDを確認してください。",
	vpn.FailureIKEServerUnverified: "VPNサーバーの証明書を検証できませんでした。サーバーのIDとCAの証明書を確認してください。",
	vpn.FailureIKEProposalMismatch: "暗号スイートのネゴシエーションに失敗しました。IKEとESPの暗号スイートを確認してください。",
	vpn.FailureIKENoResponse: "VPNサーバーから応答がありません。サーバーの指定と、UDPの500番と4500番でVPNサーバーに" +
		"到達できるかを確認してください。",
	vpn.FailureXFRMInterface: "IPsecのXFRMインターフェースを作成できませんでした。DockerのLinuxカーネルが" +
		"対応していない可能性があります。",
}

// ikev2EnglishRouteReasons は、ikev2RouteReasons の英語である。
var ikev2EnglishRouteReasons = map[vpn.FailureReason]string{
	vpn.FailureIKEAuthentication: "IKEv2 authentication failed. Check the username, the password, " +
		"the pre-shared key and the IDs.",
	vpn.FailureIKEServerUnverified: "The VPN server's certificate could not be verified. " +
		"Check the server ID and the CA certificate.",
	vpn.FailureIKEProposalMismatch: "The cipher suite negotiation failed. Check the IKE and ESP proposals.",
	vpn.FailureIKENoResponse: "The VPN server does not answer. Check the server and that UDP ports 500 and 4500 " +
		"reach it.",
	vpn.FailureXFRMInterface: "The IPsec XFRM interface could not be created. Docker's Linux kernel may not support it.",
}

// init は、IKEv2/IPsec に固有の理由を、共通の言い方の表に加える。文は Sentence が
// まとめて組み立てる。
func init() {
	for reason, sentence := range ikev2RouteReasons {
		routeReasons[reason] = sentence
	}
	for reason, sentence := range ikev2EnglishRouteReasons {
		englishRouteReasons[reason] = sentence
	}
}
