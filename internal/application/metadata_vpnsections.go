package application

import "sshc/internal/vpn"

// clearForeignVPNSections は、v8 までの VPN プロファイルから、backend と違う節を外す。
//
// v0.38.0 は API に送られた節をそのまま保存した。v0.38.1 から v0.41.0 までは、その節を
// 読むたびに無視していた。v9 からは、その節を断る（vpn.Profile.Validate）。残すと、その
// プロファイルがあるあいだ metadata を書けなくなる。
//
// v9 より後に足す方式の節は、v8 までの文書に無いので、ここに足さない。
func clearForeignVPNSections(metadata *Metadata) {
	for index := range metadata.VPNProfiles {
		stored := &metadata.VPNProfiles[index]
		if stored.Backend != vpn.WireGuard {
			stored.WireGuard = nil
		}
		if stored.Backend != vpn.L2TPIPsec {
			stored.L2TP = nil
		}
		if stored.Backend != vpn.OpenConnect {
			stored.OpenConnect = nil
		}
		if stored.Backend != vpn.OpenVPN {
			stored.OpenVPN = nil
		}
		if stored.Backend != vpn.IKEv2 {
			stored.IKEv2 = nil
		}
	}
}
