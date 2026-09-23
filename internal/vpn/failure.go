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
	// FailureTargetUnresolved は、VPN の中で接続先の名前を引けなかったことを表す。
	FailureTargetUnresolved FailureReason = "target_unresolved"
	// FailureTunnelLost は、用意できたあとでトンネルが落ちたことを表す。
	FailureTunnelLost FailureReason = "tunnel_lost"
)

// knownFailureReasons は、agent が書いてよい語である。知らない語は読まない。
var knownFailureReasons = map[FailureReason]bool{
	FailureUnknown: true, FailureTimeout: true, FailureServerUnresolved: true,
	FailureIPsecNegotiation: true, FailurePPPAuthentication: true, FailureOpenConnect: true,
	FailureHandshakeTimeout: true, FailureTargetUnresolved: true, FailureTunnelLost: true,
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
