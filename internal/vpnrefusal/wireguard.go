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

// wireGuardEnglishDirectiveReasons は、wireGuardDirectiveReasons の英語である。
var wireGuardEnglishDirectiveReasons = map[vpn.Reason]string{
	vpn.ReasonNotIPv4:            "\"%s\" has no IPv4 address.",
	vpn.ReasonUnroutable:         "\"%s\" has an address that cannot be used (loopback, multicast or unspecified).",
	vpn.ReasonOutOfRange:         "the port in \"%s\" must be between 0 and 65535.",
	vpn.ReasonDuplicate:          "\"%s\" appears more than once. Keep only one.",
	vpn.ReasonMisplacedDirective: "\"%s\" cannot be written in this section.",
	vpn.ReasonNotInAllowedIPs:    "the address in \"%s\" is outside the AllowedIPs of every [Peer].",
}

// wireGuardEnglishFieldReasons は、wireGuardFieldReasons の英語である。
var wireGuardEnglishFieldReasons = map[vpn.Reason]string{
	vpn.ReasonUnknownDirective:   "This item cannot be used in a WireGuard configuration file.",
	vpn.ReasonMisplacedDirective: "This item cannot be written in this section.",
	vpn.ReasonMissingDirective:   "A required item is missing.",
	vpn.ReasonDuplicate:          "The same item appears more than once.",
	vpn.ReasonNoEndpoint:         "The configuration file names no VPN server (Endpoint).",
	vpn.ReasonNotInAllowedIPs:    "A DNS server is outside the AllowedIPs of every [Peer].",
	vpn.ReasonKeepaliveTooLong: "PersistentKeepalive must be %d seconds or less; " +
		"sshc tells a dropped VPN by whether handshakes keep coming.",
	vpn.ReasonMTUOutOfRange: "MTU must be between 576 and 65535.",
}

// wireGuardEnglishTargetReasons は、wireGuardTargetReasons の英語である。
var wireGuardEnglishTargetReasons = map[vpn.FailureReason]string{
	vpn.FailureTargetNotAllowed: "The destination is outside the AllowedIPs of every WireGuard [Peer]. " +
		"Check AllowedIPs in the configuration file and the connection's HostName.",
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
	for reason, pattern := range wireGuardEnglishDirectiveReasons {
		englishDirectiveReasons[reason] = pattern
	}
	for reason, sentence := range wireGuardEnglishFieldReasons {
		englishFieldReasons[reason] = sentence
	}
	for reason, sentence := range wireGuardEnglishTargetReasons {
		englishTargetReasons[reason] = sentence
	}
}
