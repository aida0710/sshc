package vpn

import (
	"strconv"
	"strings"
)

// openconnect backend の設定を組み立てる。
//
// openconnect は接続のたびに外部スクリプトを呼び、そこで interface と経路を
// 作らせる。既定の vpnc-script は装置が配った既定経路とDNSをそのまま入れる。
// それでは、このコンテナが「接続先ひとつだけを通す」約束を守れない。だから
// 自分の script を渡し、interface を上げるところまでで止める。

// defaultOpenConnectProtocol は、方式を書かなかったときに使う方式である。
// Cisco AnyConnect と、それに合わせた装置（ocserv など）が話す。
const defaultOpenConnectProtocol = "anyconnect"

// defaultTunnelMTU は、装置がMTUを配らなかったときに使う大きさである。
const defaultTunnelMTU = 1400

// openConnectScript は、openconnect が呼ぶ script の本文である。
//
// 受け取る値は openconnect が環境変数で渡す。reason が connect のときだけ
// interface を用意し、それ以外（pre-init、disconnect、reconnect）では何もしない。
func openConnectScript() string {
	return strings.Join([]string{
		"#!/bin/sh",
		"# sshc がこの経路のために渡す script である。装置が配った既定経路とDNSは",
		"# 入れない。接続先ひとつぶんの経路は agent が作る。",
		"set -eu",
		`case "${reason:-}" in`,
		"connect)",
		`	ip link set dev "$TUNDEV" up mtu "${INTERNAL_IP4_MTU:-` + strconv.Itoa(defaultTunnelMTU) + `}"`,
		`	ip address add "$INTERNAL_IP4_ADDRESS/32" dev "$TUNDEV"`,
		"	;;",
		"esac",
		"exit 0",
		"",
	}, "\n")
}

// openConnectProtocol は、profile が指定した方式、または既定を返す。
func openConnectProtocol(settings OpenConnectSettings) string {
	if settings.Protocol == "" {
		return defaultOpenConnectProtocol
	}
	return settings.Protocol
}
