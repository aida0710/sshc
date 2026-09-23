package vpnprofile

import "sshc/internal/vpn"

// overlaySecrets は、保存済みの秘密に、送られた項目だけを重ねる。
//
// 空の項目は「送られていない」であり、保存済みの値を残す。画面と CLI は保存済みの
// 秘密を読み出せないので、設定だけを直すときは秘密を空のまま送ってくる。
func overlaySecrets(stored, sent vpn.SecretsDocument) vpn.SecretsDocument {
	merged := stored
	for _, field := range []struct {
		target *string
		value  string
	}{
		{&merged.WireGuardPrivateKey, sent.WireGuardPrivateKey},
		{&merged.L2TPPassword, sent.L2TPPassword},
		{&merged.IPsecPSK, sent.IPsecPSK},
		{&merged.OpenConnectPassword, sent.OpenConnectPassword},
		{&merged.OpenConnectTOTPSecret, sent.OpenConnectTOTPSecret},
	} {
		if field.value != "" {
			*field.target = field.value
		}
	}
	return merged
}
