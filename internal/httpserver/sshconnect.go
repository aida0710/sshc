package httpserver

import (
	"context"
	"errors"

	"sshc/internal/application"
	"sshc/internal/keys"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/vpn"
	"sshc/internal/vpnrefusal"
)

// problemVPNRouteDisconnected は、利用者が切断した VPN 経路のために、自動再接続を
// 止めたことを表す語である。
const problemVPNRouteDisconnected = "vpn_route_disconnected"

// reconnectStopNotice は、再接続を止めた理由のうち、既定の文（設定を直すよう促す）が
// 当てはまらないものについて、ターミナルへ書く文を返す。
func reconnectStopNotice(problem string) string {
	if problem == problemVPNRouteDisconnected {
		return "VPN経路が切断されたため、自動再接続を停止しました。"
	}
	return ""
}

// Connector は、alias ひとつ分の対話セッションを開く。
//
// 外部の ssh は起こさない。組み立てるのは合成の根（internal/app）であり、
// 鍵も vault も known_hosts もそこで一度だけ配線される。二箇所で組み立てると、
// 片方だけが vault を見る日が来る。
type Connector func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error)

// connectProblem は、接続を組み立てられなかった理由を通信形式に変える。
func connectProblem(err error) (string, bool) {
	var unresolvable *application.ErrUnresolvable
	switch {
	case errors.As(err, &unresolvable):
		return "alias_unresolvable", true
	case errors.Is(err, sshclient.ErrJumpDepth):
		return "jump_depth_exceeded", true
	case errors.Is(err, sshclient.ErrNoHostName):
		return "alias_unresolvable", true
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
	case errors.Is(err, sshclient.ErrProxyAuthenticationRequired):
		return "proxy_authentication_required", true
	case errors.Is(err, sshclient.ErrPromptAborted):
		return "authentication_cancelled", true
	case errors.Is(err, keys.ErrPassphraseRequired), errors.Is(err, keys.ErrWrongPassphrase):
		return "key_passphrase_required", true
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
