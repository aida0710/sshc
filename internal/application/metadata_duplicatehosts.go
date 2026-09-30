package application

// schema 9 より前の sshc は、接続の改名や保存先の移動で entry を付け直すとき、行き先に
// 残っていた entry（多くは接続先が消えた orphan）を消さなかった。そのため、同じ接続の
// entry が 2 つある metadata.json がある。schema 9 からは同じ接続の entry を 2 つ持たない
// （ValidateMetadata が断る）。2 つある前の形を読むのは、schema 9 より前の metadata を
// 移行するときだけである（DecodeMetadata）。

// keepOneEntryPerConnection は、同じ接続の entry を 1 つにする。
//
// 付け直した entry は orphan の印を外してあるので、印の無い entry を残す。印で決まらない
// ときは先頭を残す。前の sshc は、接続するときの文字コードと VPN プロファイルに先頭の
// entry を使っていた。
func keepOneEntryPerConnection(metadata *Metadata) {
	positions := make(map[HostIdentity]int, len(metadata.Hosts))
	kept := make([]HostMetadata, 0, len(metadata.Hosts))
	for _, host := range metadata.Hosts {
		position, seen := positions[host.Identity]
		if !seen {
			positions[host.Identity] = len(kept)
			kept = append(kept, host)
			continue
		}
		if kept[position].Orphan && !host.Orphan {
			kept[position] = host
		}
	}
	metadata.Hosts = kept
}
