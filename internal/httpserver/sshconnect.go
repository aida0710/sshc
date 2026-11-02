package httpserver

import (
	"errors"

	"sshc/internal/application"
	"sshc/internal/keys"
	"sshc/internal/sshclient"
	"sshc/internal/vpn"
	"sshc/internal/vpnrefusal"
)

// problemVPNRouteDisconnected は、利用者が切断した VPN 経路のために、自動再接続を
// 止めたことを表す語である。VPN の操作が断るときの語と同じ語を使う。
const problemVPNRouteDisconnected = vpnrefusal.CodeRouteDisconnected

// reconnectStopNotice は、再接続を止めた理由のうち、既定の文（設定を直すよう促す）が
// 当てはまらないものについて、ターミナルへ書く文を返す。
func reconnectStopNotice(problem string) string {
	if problem == problemVPNRouteDisconnected {
		return "VPN経路が切断されたため、自動再接続を停止しました。"
	}
	return ""
}

// connectProblem は、接続を組み立てられなかった理由を通信形式に変える。
func connectProblem(err error) (string, bool) {
	var unresolvable *application.ErrUnresolvable
	var symlinkedKnownHosts *sshclient.KnownHostsSymlinkError
	switch {
	case errors.As(err, &unresolvable):
		return "alias_unresolvable", true
	case errors.As(err, &symlinkedKnownHosts):
		return problemKnownHostsSymlink, true
	case errors.Is(err, sshclient.ErrJumpDepth):
		return "jump_depth_exceeded", true
	case errors.Is(err, sshclient.ErrNoHostName):
		return "alias_unresolvable", true
	case errors.Is(err, sshclient.ErrHostKeyDeclined):
		return "host_key_declined", true
	case errors.Is(err, sshclient.ErrHostKeyUnknown):
		return "host_key_unknown", true
	case errors.Is(err, sshclient.ErrHostKeyChanged):
		return "host_key_changed", true
	case errors.Is(err, sshclient.ErrHostKeyRevoked):
		return "host_key_revoked", true
	case errors.Is(err, sshclient.ErrNoIdentity):
		return "identity_unavailable", true
	case errors.Is(err, sshclient.ErrNoAuthMethod):
		return "authentication_unavailable", true
	case errors.Is(err, sshclient.ErrAuthenticationRejected):
		return "authentication_rejected", true
	case errors.Is(err, sshclient.ErrProxyAuthenticationRequired):
		return "proxy_authentication_required", true
	case errors.Is(err, sshclient.ErrPromptAborted):
		return "authentication_cancelled", true
	case errors.Is(err, keys.ErrPassphraseRequired), errors.Is(err, keys.ErrWrongPassphrase):
		return "key_passphrase_required", true
	case errors.Is(err, sshclient.ErrVPNWithProxyCommand),
		errors.Is(err, sshclient.ErrVPNThroughJump),
		errors.Is(err, sshclient.ErrProxyCommandThroughJump):
		// 経路の組み合わせが矛盾している。設定を直さない限り毎回同じ理由で断る。
		return "route_misconfigured", true
	}
	// 利用者が切断した VPN 経路は、自動再接続では起動し直さない。
	if errors.Is(err, vpn.ErrRouteDisconnected) {
		return problemVPNRouteDisconnected, true
	}
	// VPN の経路を用意できない理由のうち、設定を直さない限り同じ理由で断られる
	// ものは、再接続を繰り返さない。理由の文は接続ログに出ている。
	if refusal, known := vpnrefusal.Of(err); known && refusal.RequiresAction() {
		return "vpn_route_refused", true
	}
	return "", false
}
