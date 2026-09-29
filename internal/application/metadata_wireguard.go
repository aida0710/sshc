package application

// metadata の schema 8 で変わった、WireGuard のプロファイルの読み方。
//
// schema 8 からの WireGuard のプロファイルは、設定ファイル（鍵を含む）を Vault に置き、metadata
// には Endpoint のサーバー（servers）だけを持つ。v0.40.0 までの項目の形（server、peerPublicKey、
// address）で保存したプロファイルは、保存し直すまで項目のまま読む。設定ファイルを組み立てる
// には Vault の秘密鍵が要り、metadata を読む時点では Vault が開いていないことがあるからである。

// fillWireGuardServers は、項目の形の WireGuard のプロファイルに、項目の server から
// サーバーを補う。一覧は、どちらの形でも servers を出す。
func fillWireGuardServers(metadata *Metadata) {
	for index := range metadata.VPNProfiles {
		stored := &metadata.VPNProfiles[index]
		fields, found := stored.WireGuardFields()
		if !found || len(stored.WireGuard.Servers) > 0 {
			continue
		}
		stored.WireGuard.Servers = fields.Servers()
	}
}
