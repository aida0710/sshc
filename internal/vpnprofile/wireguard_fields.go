package vpnprofile

import (
	"sshc/internal/application"
	"sshc/internal/vpn"
)

// v0.40.0 までの項目の形（サーバー、相手の公開鍵、トンネルのアドレスと、Vault の秘密鍵）で
// 保存した WireGuard のプロファイルを、設定ファイルの形の秘密として読む。
//
// 移すのは、利用者が保存し直したときである。読むたびに項目と秘密鍵から設定ファイルを組み立て、
// 経路の起動と編集の画面には設定ファイルの形だけを見せる。保存すると、組み立てた（または
// 利用者が直した）設定ファイルを Vault に、サーバーを metadata に書き、項目と秘密鍵は消える。
// Vault が開いたときに黙って書き換えないのは、書き込みを利用者の操作の外で起こさないため
// である。

// readSecrets は、Vault の記録を、プロファイルの方式の秘密として読む。
func readSecrets(stored application.VPNProfile, record string) (vpn.Secrets, error) {
	secrets, err := vpn.DecodeSecrets(record)
	if err != nil {
		return vpn.Secrets{}, err
	}
	fields, found := stored.WireGuardFields()
	if !found || secrets.WireGuard != nil {
		return secrets, nil
	}
	privateKey, err := vpn.DecodeWireGuardFieldsPrivateKey(record)
	if err != nil || privateKey == "" {
		return secrets, err
	}
	secrets.WireGuard = &vpn.WireGuardSecrets{Config: fields.Config(privateKey)}
	return secrets, nil
}
