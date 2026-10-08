package vpn

import (
	"encoding/hex"
	"strings"
)

// strongSwan を使う backend（l2tp_ipsec と ikev2）が共通に使うもの。
//
// どちらも、strongSwan などが読む本文を engine が組み立て、agent はそれを置いて、
// VPN サーバーのアドレスだけを自分で名前解決して埋める。IPsec は相手のアドレスを
// 設定に書くので、名前解決する場所が違えば別のサーバーへ接続しうる。

// serverAddressPlaceholder は、agent が名前解決したアドレスで置き換える印である。
const serverAddressPlaceholder = "%SERVER_ADDRESS%"

// connectionName は、strongSwan（L2TP/IPsec では xl2tpd も）がこの接続を指す名前である。
// agent の手順（container/backend-*.sh の connection）も同じ名前を使う。
const connectionName = "sshc-vpn"

// strongSwanDocument は、agent が置くだけの本文と、agent が自分で名前解決する相手である。
type strongSwanDocument struct {
	// Server は、VPNサーバーの名前またはアドレスである。agent がコンテナの中で
	// 名前解決し、本文の中の印を、そのアドレスで置き換える。
	Server string `json:"server"`
	// Documents は、ファイル名から本文への対応である。
	Documents map[string]string `json:"documents"`
}

// strongSwanDaemonConfiguration は、charon が読む strongswan.conf を作る。イメージの
// 既定の設定を読み込み、charon の節に charonSettings（1段下げた行）を足して、ログを
// runtime の logFile へ書かせる。
//
// charon の既定のログの書き先は syslog で、コンテナの中には受け取る相手がいない。
// agent は失敗の理由と、失敗したときに見せるログを、このファイルから読む。
func strongSwanDaemonConfiguration(logFile string, charonSettings []string) string {
	lines := append([]string{"include /etc/strongswan.conf", "charon {"}, charonSettings...)
	lines = append(lines,
		"    filelog {",
		"        sshc {",
		"            path = "+agentRuntimeDirectory+"/"+logFile,
		"            default = 1",
		"            time_format = %H:%M:%S",
		// 書きためられると、止めたときに失われ、agent も途中で読めない。
		"            flush_line = yes",
		"        }",
		"    }",
		"}",
		"",
	)
	return strings.Join(lines, "\n")
}

// validateProposals は、IKE と ESP の暗号スイートを確かめる。どちらも任意で、
// 設定ファイルへそのまま書くので、空白・引用符・backslash を含むものは断る。
//
// section は、保存形式での節の名前（`l2tp`、`ikev2`）である。
func validateProposals(section, ike, esp string) error {
	for _, proposal := range []struct{ field, value string }{
		{section + ".ike", ike}, {section + ".esp", esp},
	} {
		if err := validateLength(proposal.field, proposal.value, maxProposalLength); err != nil {
			return err
		}
		if strings.ContainsAny(proposal.value, " \t\r\n\"\\") {
			return fieldError(ErrSettings, proposal.field, ReasonFormat)
		}
	}
	return nil
}

// strongSwanSecret は、strongSwan の設定へ秘密を書く形である。16 進で書くと、
// 引用符や backslash を含む秘密でも、strongSwan の構文として解釈されない。
func strongSwanSecret(value string) string {
	return "0x" + hex.EncodeToString([]byte(value))
}

// secretAndHexForms は、秘密と、その16進表記（小文字と大文字）である。
// strongSwanSecret で書いた秘密は、その形のままログに現れうるので、どれも伏せる。
func secretAndHexForms(value string) []string {
	if value == "" {
		return nil
	}
	encoded := hex.EncodeToString([]byte(value))
	return []string{value, encoded, strings.ToUpper(encoded)}
}
