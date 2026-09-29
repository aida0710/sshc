package vpn

import "fmt"

// FailureReason は、コンテナが経路を用意できなかった理由の語である。
//
// agent が決まった語だけを書き、engine はそれを読んで返す。画面と CLI は語を
// 翻訳して見せる。生のログは IP アドレスやパスを含むので、応答には載せない。
type FailureReason string

const (
	// FailureUnknown は、理由を読めなかったことを表す。ログを見るよう案内する。
	FailureUnknown FailureReason = "unknown"
	// FailureTimeout は、engine が待つ上限までに経路ができなかったことを表す。
	FailureTimeout FailureReason = "timeout"
	// FailureServerUnresolved は、VPN 装置の名前を引けなかったことを表す。
	FailureServerUnresolved FailureReason = "server_unresolved"
	// FailureIPsecNegotiation は、IPsec が成立しなかったことを表す（事前共有鍵、暗号方式）。
	FailureIPsecNegotiation FailureReason = "ipsec_negotiation"
	// FailurePPPAuthentication は、PPP の認証が通らなかったことを表す（利用者名、パスワード）。
	FailurePPPAuthentication FailureReason = "ppp_authentication"
	// FailureOpenConnect は、openconnect が装置へ繋げなかったことを表す。
	FailureOpenConnect FailureReason = "openconnect_failed"
	// FailureHandshakeTimeout は、WireGuard の相手と握手できなかったことを表す（鍵、サーバー）。
	FailureHandshakeTimeout FailureReason = "handshake_timeout"
	// FailureTargetUnresolved は、VPN の中で接続先の名前解決に失敗したことを表す。
	FailureTargetUnresolved FailureReason = "target_unresolved"
	// FailureTargetNeedsDNS は、接続先を名前で書いたのに、プロファイルに DNS
	// サーバーが無いことを表す。
	FailureTargetNeedsDNS FailureReason = "target_needs_dns"
	// FailureTargetIsServer は、接続先が VPN サーバーそのものであることを表す。
	FailureTargetIsServer FailureReason = "target_is_server"
	// FailureTargetUnreachable は、VPN の中で接続先へ繋げなかったことを表す
	// （接続の拒否、応答なし、経路なし）。
	FailureTargetUnreachable FailureReason = "target_unreachable"
	// FailureTargetNotAllowed は、接続先が WireGuard のどの Peer の AllowedIPs にも含まれない
	// ことを表す。WireGuard はその接続先へのパケットをどの Peer にも送らない。
	FailureTargetNotAllowed FailureReason = "target_not_allowed"
	// FailureTunnelLost は、用意できたあとでトンネルが落ちたことを表す。
	FailureTunnelLost FailureReason = "tunnel_lost"
	// FailureOpenVPNAuthentication は、OpenVPN のサーバーが認証を拒否したこと（AUTH_FAILED）を
	// 表す（ユーザー名、パスワード）。
	FailureOpenVPNAuthentication FailureReason = "openvpn_authentication"
	// FailureOpenVPNTLS は、OpenVPN の TLS のハンドシェイクに失敗したことを表す（証明書の
	// 検証、鍵）。
	FailureOpenVPNTLS FailureReason = "openvpn_tls"
	// FailureOpenVPNNoResponse は、OpenVPN のサーバーから応答が無かったことを表す（サーバーの
	// 指定、ネットワーク、tls-auth と tls-crypt の鍵）。
	FailureOpenVPNNoResponse FailureReason = "openvpn_no_response"
	// FailureOpenVPNConfiguration は、OpenVPN が設定ファイルを読み込めなかったことを表す。
	FailureOpenVPNConfiguration FailureReason = "openvpn_configuration"
	// FailureOpenVPN は、OpenVPN がほかの理由で接続できなかったことを表す。
	FailureOpenVPN FailureReason = "openvpn_failed"
	// FailureIKEAuthentication は、VPN サーバーがこちらの認証を拒否したことを表す
	// （IKEv2 のユーザー名とパスワード、事前共有鍵、ID）。
	FailureIKEAuthentication FailureReason = "ike_authentication"
	// FailureIKEServerUnverified は、VPN サーバーを確かめられなかったことを表す
	// （サーバーの証明書を信頼できない、サーバーの ID が違う）。
	FailureIKEServerUnverified FailureReason = "ike_server_unverified"
	// FailureIKEProposalMismatch は、IKE または ESP の暗号スイートが VPN サーバーと
	// 合わなかったことを表す。
	FailureIKEProposalMismatch FailureReason = "ike_proposal_mismatch"
	// FailureIKENoResponse は、VPN サーバーが応答しなかったことを表す（サーバーの
	// 指定、UDP の 500 番と 4500 番の到達性）。
	FailureIKENoResponse FailureReason = "ike_no_response"
	// FailureXFRMInterface は、IPsec の XFRM インターフェースを作れなかったことを
	// 表す。Docker の Linux カーネルが対応していない。
	FailureXFRMInterface FailureReason = "xfrm_interface_unavailable"
)

// knownFailureReasons は、agent が書いてよい語である。知らない語は読まない。
var knownFailureReasons = map[FailureReason]bool{
	FailureUnknown: true, FailureTimeout: true, FailureServerUnresolved: true,
	FailureIPsecNegotiation: true, FailurePPPAuthentication: true, FailureOpenConnect: true,
	FailureHandshakeTimeout: true, FailureTargetUnresolved: true, FailureTargetNeedsDNS: true,
	FailureTargetIsServer: true, FailureTargetUnreachable: true, FailureTunnelLost: true,
	FailureOpenVPNAuthentication: true, FailureOpenVPNTLS: true, FailureOpenVPNNoResponse: true,
	FailureOpenVPNConfiguration: true, FailureOpenVPN: true,
	FailureIKEAuthentication: true, FailureIKEServerUnverified: true, FailureIKEProposalMismatch: true,
	FailureIKENoResponse: true, FailureXFRMInterface: true,
	FailureTargetNotAllowed: true,
}

// SessionFailure は、経路を用意できなかったことと、その理由である。
//
// errors.Is(err, ErrSessionFailed) でも見分けられる。
type SessionFailure struct {
	Profile string
	Reason  FailureReason
}

func (failure *SessionFailure) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrSessionFailed, failure.Profile, failure.Reason)
}

func (failure *SessionFailure) Unwrap() error { return ErrSessionFailed }
