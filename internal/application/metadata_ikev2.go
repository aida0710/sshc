package application

import "sshc/internal/vpn"

// clearUnpinnedServerIdentities は、v8 までの IKEv2 のプロファイルから、どのサーバーにも
// 一致するサーバーの ID（`*`、`0.0.0.0` など）を外す。外すと、VPN サーバーの名前を ID に
// 使う。
//
// v9 からは、この ID を保存できない（vpn.ServerIdentityMatchesManyServers）。残すと、その
// プロファイルがあるあいだ metadata を書けなくなる。
func clearUnpinnedServerIdentities(metadata *Metadata) {
	for index := range metadata.VPNProfiles {
		settings := metadata.VPNProfiles[index].IKEv2
		if settings != nil && vpn.ServerIdentityMatchesManyServers(settings.ServerIdentity) {
			settings.ServerIdentity = ""
		}
	}
}
