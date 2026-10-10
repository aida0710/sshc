package vpn

import (
	"fmt"
	"strings"
)

// ikev2 backend が agent へ渡す文書。charon の設定（strongswan.conf）と、接続と
// 秘密（swanctl.conf）と、指定した CA の証明書を組み立てる。

// ikev2VirtualIPRequest は、サーバーに仮想 IP（IPv4）を配ってもらう指定である。
// ipsec.conf の leftsourceip=%config にあたる。
const ikev2VirtualIPRequest = "0.0.0.0"

// ikev2DPDDelay は、サーバーの無応答を確かめる間隔である。L2TP/IPsec と同じにする。
const ikev2DPDDelay = "30s"

// ikev2Document は、agent が IKEv2 のトンネルを張るのに要るものである。
type ikev2Document struct {
	strongSwanDocument
	// Link は、agent が charon を起動する前に作る XFRM インターフェースである。
	Link xfrmInterface `json:"link"`
	// PublicAuthorities は、公的な認証局の証明書（ca-certificates）を信頼するかで
	// ある。CA を指定したときは、その CA だけを信頼する。
	PublicAuthorities bool `json:"publicAuthorities,omitempty"`
}

// ikev2Documents は、コンテナへ渡す本文を返す。charon の設定（strongswan.conf）、
// 接続と秘密（swanctl.conf）、指定した CA の証明書（ca-<番号>.pem）である。
func ikev2Documents(settings IKEv2Settings, secrets IKEv2Secrets) (map[string]string, error) {
	documents := map[string]string{
		"strongswan.conf": ikev2DaemonConfiguration(),
	}
	var authorities []string
	if settings.CACertificate != "" {
		certificates, err := caCertificateBlocks(settings.CACertificate)
		if err != nil {
			return nil, fieldError(ErrSettings, IKEv2CACertificateField, ReasonFormat)
		}
		for index, certificate := range certificates {
			name := fmt.Sprintf("ca-%d.pem", index+1)
			documents[name] = certificate
			authorities = append(authorities, agentRuntimeDirectory+"/"+name)
		}
	}
	documents["swanctl.conf"] = ikev2SwanctlConfiguration(settings, secrets, authorities)
	return documents, nil
}

// ikev2DaemonConfiguration は、charon が読む strongswan.conf を作る。イメージの
// 既定の設定を読み込み、この経路に要る違いだけを上書きする。
func ikev2DaemonConfiguration() string {
	return strongSwanDaemonConfiguration("charon.log", []string{
		// 経路は agent が接続先ごとに作る。サーバーが配るトラフィックセレクターを
		// 経路表へ入れさせると、コンテナのほかの通信までトンネルへ向かう。
		"    install_routes = no",
		// 配られた仮想 IP は XFRM インターフェースに付ける。接続先への経路の
		// 送信元がこのアドレスになる。
		"    install_virtual_ip_on = " + ikev2Link.Name,
		"    plugins {",
		// サーバーが配る DNS をコンテナの resolv.conf へ書かせない。VPN の中の
		// DNS はプロファイルに書いたものだけを使う。
		"        resolve {",
		"            load = no",
		"        }",
		// 接続は swanctl（vici）で渡す。ipsec.conf と /etc/ipsec.d は読ませない。
		"        stroke {",
		"            load = no",
		"        }",
		"    }",
	})
}

// ikev2SwanctlConfiguration は、swanctl が読む接続と秘密を作る。authorities は、
// サーバーの証明書を確かめる CA の証明書のパスである。空なら、agent が読み込んだ
// 公的な認証局で確かめる。
//
// 値は引用符で囲んで書く。引用符の中は、`#` や `${` を含めて文字どおりに読まれる。
// 引用符と backslash と改行は、検査で断ってある。
func ikev2SwanctlConfiguration(settings IKEv2Settings, secrets IKEv2Secrets, authorities []string) string {
	serverIdentity := settings.ServerIdentity
	if serverIdentity == "" {
		serverIdentity = settings.Server
	}
	connection := []string{
		"version = 2",
		"remote_addrs = " + serverAddressPlaceholder,
		"vips = " + ikev2VirtualIPRequest,
		// NAT の内側から接続することが多い。UDP 4500 へ包む。
		"encap = yes",
		"keyingtries = 1",
		"dpd_delay = " + ikev2DPDDelay,
	}
	if settings.IKE != "" {
		connection = append(connection, "proposals = "+swanctlQuote(swanctlProposals(settings.IKE)))
	}
	local := []string{"id = " + swanctlQuote(settings.Identity)}
	// サーバーの ID は必ず書く。書かなければ、信頼する認証局の証明書を持つ誰とでも
	// 繋いでしまう。
	remote := []string{"id = " + swanctlQuote(serverIdentity)}
	var secret []string
	if settings.Authentication == IKEv2AuthenticationPSK {
		local = append(local, "auth = psk")
		remote = append(remote, "auth = psk")
		secret = swanctlBlock("ike-"+connectionName, []string{"secret = " + strongSwanSecret(secrets.PreSharedKey)})
	} else {
		local = append(local, "auth = eap-mschapv2", "eap_id = "+swanctlQuote(settings.Identity))
		remote = append(remote, "auth = pubkey")
		if len(authorities) > 0 {
			remote = append(remote, "cacerts = "+strings.Join(authorities, ","))
		}
		secret = swanctlBlock("eap-"+connectionName, []string{
			"id = " + swanctlQuote(settings.Identity),
			"secret = " + strongSwanSecret(secrets.Password),
		})
	}
	child := []string{
		// こちらはサーバーの向こう全体を求め、サーバーが許す範囲に狭めさせる。
		// ポリシーは if_id でインターフェースに結び付くので、既定経路は奪わない。
		"remote_ts = 0.0.0.0/0",
		fmt.Sprintf("if_id_in = %d", ikev2Link.ID),
		fmt.Sprintf("if_id_out = %d", ikev2Link.ID),
		"dpd_action = clear",
	}
	if settings.ESP != "" {
		child = append(child, "esp_proposals = "+swanctlQuote(swanctlProposals(settings.ESP)))
	}
	connection = append(connection, swanctlBlock("local", local)...)
	connection = append(connection, swanctlBlock("remote", remote)...)
	connection = append(connection, swanctlBlock("children", swanctlBlock(connectionName, child))...)
	lines := swanctlBlock("connections", swanctlBlock(connectionName, connection))
	lines = append(lines, swanctlBlock("secrets", secret)...)
	return strings.Join(lines, "\n") + "\n"
}

// swanctlBlock は、name の節を、中身の行を1段下げて返す。
func swanctlBlock(name string, body []string) []string {
	lines := make([]string, 0, len(body)+2)
	lines = append(lines, name+" {")
	for _, line := range body {
		lines = append(lines, "    "+line)
	}
	return append(lines, "}")
}

// swanctlQuote は、値を swanctl.conf の文字列として書ける形にする。
func swanctlQuote(value string) string { return `"` + value + `"` }

// swanctlProposals は、L2TP/IPsec（ipsec.conf）と同じ書き方の暗号スイートを、
// swanctl.conf の書き方へ直す。
//
// ipsec.conf では、指定した候補のあとに strongSwan の既定の候補も提示し、末尾に
// `!` を付けたときだけ指定した候補に限る。swanctl.conf は指定した候補だけを提示
// するので、`!` が無ければ既定（default）を後ろに足す。どちらの方式でも、同じ値が
// 同じ意味になる。
func swanctlProposals(value string) string {
	if strict, found := strings.CutSuffix(value, "!"); found {
		return strict
	}
	return value + ",default"
}
