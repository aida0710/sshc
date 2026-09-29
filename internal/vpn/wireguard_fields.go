package vpn

import (
	"fmt"
	"strings"
)

// v0.40.0 までの WireGuard のプロファイルは、設定ファイルではなく、サーバー、相手の公開鍵、
// トンネルのアドレスの3つの項目で保存していた（秘密鍵は Vault の wireguardPrivateKey）。
// 保存し直すまでは、この項目と秘密鍵から設定ファイルを組み立てて使う。

// WireGuardFields は、v0.40.0 までの項目の形で保存した WireGuard のプロファイルである。
type WireGuardFields struct {
	// Server は、トンネルの相手である。`host:port` で書く。
	Server string
	// PeerPublicKey は、相手の公開鍵である。
	PeerPublicKey string
	// Address は、トンネル側でこのマシンが名乗るアドレス（CIDR）である。
	Address string
	// DNS は、プロファイルの DNS である。
	DNS []string
}

// fieldsPlaceholderPrivateKey は、項目を確かめるときに秘密鍵の代わりに置く鍵である。形だけが
// 鍵として正しい。
var fieldsPlaceholderPrivateKey = strings.Repeat("A", wireGuardKeyLength-1) + "="

// Config は、項目と秘密鍵から設定ファイルを組み立てる。
//
// 以前の形では、sshc が接続先をひとつずつ AllowedIPs に足していた。どの接続先へも届いた
// ので、AllowedIPs は 0.0.0.0/0 にする。経路は connect が接続先ごとに作るので、トンネルへ
// 流れる通信は以前と変わらない。
func (fields WireGuardFields) Config(privateKey string) string {
	lines := []string{"[Interface]", "PrivateKey = " + privateKey, "Address = " + fields.Address}
	if len(fields.DNS) > 0 {
		lines = append(lines, "DNS = "+strings.Join(fields.DNS, ", "))
	}
	lines = append(lines,
		"",
		"[Peer]",
		"PublicKey = "+fields.PeerPublicKey,
		"Endpoint = "+fields.Server,
		"AllowedIPs = 0.0.0.0/0",
		fmt.Sprintf("PersistentKeepalive = %d", defaultWireGuardKeepaliveSeconds),
	)
	return strings.Join(append(lines, ""), "\n")
}

// Servers は、組み立てる設定ファイルの Endpoint のサーバーである。
func (fields WireGuardFields) Servers() []string {
	if !strings.Contains(fields.Server, ":") {
		return nil
	}
	return []string{wireGuardEndpointHost(fields.Server)}
}

// Validate は、項目から、使える設定ファイルを組み立てられるかを確かめる。
//
// 組み立てた本文を設定ファイルと同じ規則で読むので、ここで通れば、経路を起動するときに
// 組み立てる本文も通る。秘密鍵は Vault にあるので、形だけが正しい鍵を代わりに置く。
func (fields WireGuardFields) Validate() error {
	_, err := ParseWireGuardConfig([]byte(fields.Config(fieldsPlaceholderPrivateKey)))
	return err
}
