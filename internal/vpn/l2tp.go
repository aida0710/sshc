package vpn

import (
	"encoding/hex"
	"strconv"
	"strings"
)

// l2tp_ipsec の設定を組み立てる。
//
// 中身は strongSwan、xl2tpd、pppd がそれぞれ読む本文である。コンテナの agent は
// 受け取った本文をそのまま置き、相手のアドレスだけを自分で引いて埋める。IPsec は
// 相手のアドレスを設定に書くので、名前を引く場所が違えば別の装置へ繋ぎうる。

// serverAddressPlaceholder は、agent が解決したアドレスで置き換える印である。
const serverAddressPlaceholder = "%SERVER_ADDRESS%"

// connectionName は、strongSwan と xl2tpd が使うこの接続の名前である。
const connectionName = "sshc-vpn"

// pppMTU は、PPP・L2TP・UDP・IPsec の各ヘッダを載せても 1500 に収まる大きさである。
const pppMTU = 1280

// l2tpDocuments は、コンテナへ渡す4つの本文を返す。
func l2tpDocuments(settings L2TPSettings, secrets Secrets) map[string]string {
	proposals := ""
	for _, proposal := range []struct{ field, value string }{{"ike", settings.IKE}, {"esp", settings.ESP}} {
		if proposal.value != "" {
			proposals += "    " + proposal.field + "=" + proposal.value + "\n"
		}
	}
	ipsec := strings.Join([]string{
		"config setup",
		`    charondebug="ike 1, knl 1, cfg 0"`,
		"conn " + connectionName,
		"    keyexchange=ikev1",
		"    authby=psk",
		// transport mode。L2TP は IPsec の中を通る UDP 1701 であり、
		// トンネルモードの二重化は要らない。
		"    type=transport",
		"    left=%defaultroute",
		"    leftprotoport=17/1701",
		"    right=" + serverAddressPlaceholder,
		"    rightid=%any",
		"    rightprotoport=17/1701",
		// NAT の内側から繋ぐことが多い。UDP 4500 へ包む。
		"    forceencaps=yes",
		"    keyingtries=1",
		"    dpdaction=clear",
		"    dpddelay=30s",
		"    auto=add",
		proposals,
	}, "\n")
	ppp := strings.Join([]string{
		"ipcp-accept-local",
		"ipcp-accept-remote",
		// 相手にこちらを認証させない。こちらが相手を認証するのは IPsec の仕事である。
		"noauth",
		"noccp",
		"noipdefault",
		// 既定経路を奪わせない。接続先への /32 だけを、この名前空間に作る。
		"nodefaultroute",
		"noipv6",
		"unit 0",
		"mtu " + strconv.Itoa(pppMTU),
		"mru " + strconv.Itoa(pppMTU),
		"lcp-echo-interval 30",
		"lcp-echo-failure 3",
		"hide-password",
		"logfile /run/sshc-vpn/ppp.log",
		"user " + quotePPP(settings.Username),
		"password " + quotePPP(secrets.L2TPPassword),
		"",
	}, "\n")
	return map[string]string{
		"ipsec.conf": ipsec,
		// 16 進で書く。事前共有鍵に引用符や backslash があっても、strongSwan の
		// 構文として解釈されない。
		"ipsec.secrets": ": PSK 0x" + hex.EncodeToString([]byte(secrets.IPsecPSK)) + "\n",
		"xl2tpd.conf": strings.Join([]string{
			"[global]",
			"port = 1701",
			"force userspace = no",
			"[lac " + connectionName + "]",
			"lns = " + serverAddressPlaceholder,
			"pppoptfile = /run/sshc-vpn/ppp.options",
			"ppp debug = no",
			"autodial = no",
			"redial = no",
			"length bit = yes",
			"",
		}, "\n"),
		"ppp.options": ppp,
	}
}

// quotePPP は、pppd の options ファイルの1語として書ける形にする。
func quotePPP(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
