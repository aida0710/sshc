package vpnrefusal

import "sshc/internal/vpn"

// WireGuard の設定ファイルとプロファイルを受け取れなかった理由と、VPN 経由で接続先へ
// 繋げなかった理由の言い方である。

// wireGuardDirectiveReasons は、WireGuard の設定ファイルの項目ひとつを断った理由の言い方である。
var wireGuardDirectiveReasons = map[vpn.Reason]string{
	vpn.ReasonNotIPv4:            "「%s」にIPv4アドレスがありません。",
	vpn.ReasonUnroutable:         "「%s」に、ループバックアドレスなど使用できないアドレスがあります。",
	vpn.ReasonOutOfRange:         "「%s」のポート番号は0〜65535で指定してください。",
	vpn.ReasonDuplicate:          "「%s」が重なっています。ひとつだけにしてください。",
	vpn.ReasonMisplacedDirective: "「%s」は、この節には書けません。",
	vpn.ReasonNotInAllowedIPs:    "「%s」のアドレスが、どの[Peer]のAllowedIPsにも含まれていません。",
}

// wireGuardFieldReasons は、WireGuard の設定ファイルに固有の理由の言い方である。%d を含む
// ものには上限が入る。
var wireGuardFieldReasons = map[vpn.Reason]string{
	vpn.ReasonUnknownDirective:   "WireGuardの設定ファイルで使えない項目です。",
	vpn.ReasonMisplacedDirective: "この節には書けない項目です。",
	vpn.ReasonMissingDirective:   "必要な項目がありません。",
	vpn.ReasonDuplicate:          "同じ項目が重なっています。",
	vpn.ReasonNoEndpoint:         "接続するVPNサーバー（Endpoint）が設定ファイルにありません。",
	vpn.ReasonNotInAllowedIPs:    "DNSサーバーが、どの[Peer]のAllowedIPsにも含まれていません。",
	vpn.ReasonKeepaliveTooLong: "PersistentKeepaliveは%d秒以下にしてください。" +
		"sshcはハンドシェイクが続いているかでVPNの切断を確かめます。",
	vpn.ReasonMTUOutOfRange: "MTUは576〜65535で指定してください。",
}

// wireGuardTargetReasons は、WireGuard の経路で接続先へ繋げなかった理由の言い方である。
var wireGuardTargetReasons = map[vpn.FailureReason]string{
	vpn.FailureTargetNotAllowed: "接続先が、WireGuardのどの[Peer]のAllowedIPsにも含まれていません。" +
		"設定ファイルのAllowedIPsと、接続のHostNameを確認してください。",
}

func init() {
	for reason, pattern := range wireGuardDirectiveReasons {
		directiveReasons[reason] = pattern
	}
	for reason, sentence := range wireGuardFieldReasons {
		fieldReasons[reason] = sentence
	}
	for reason, sentence := range wireGuardTargetReasons {
		targetReasons[reason] = sentence
	}
}
