package vpnprofile

import "sshc/internal/vpn"

// overlaySecrets は、保存済みの秘密に、送られた項目だけを重ねる。
//
// 空の項目は「送られていない」であり、保存済みの値を残す。CLI は保存済みの秘密を
// 読み出さないので、設定だけを直すときは秘密を空のまま送ってくる。画面も、保存済みの秘密を
// 取り出せなかったときは空のまま送る。
func overlaySecrets(stored, sent vpn.SecretsDocument) vpn.SecretsDocument {
	merged := stored
	for _, field := range []struct {
		target *string
		value  string
	}{
		{&merged.WireGuardConfig, sent.WireGuardConfig},
		{&merged.L2TPPassword, sent.L2TPPassword},
		{&merged.IPsecPSK, sent.IPsecPSK},
		{&merged.OpenConnectPassword, sent.OpenConnectPassword},
		{&merged.OpenConnectTOTPSecret, sent.OpenConnectTOTPSecret},
		{&merged.OpenVPNConfig, sent.OpenVPNConfig},
		{&merged.OpenVPNPassword, sent.OpenVPNPassword},
		{&merged.IKEv2Password, sent.IKEv2Password},
		{&merged.IKEv2PSK, sent.IKEv2PSK},
	} {
		if field.value != "" {
			*field.target = field.value
		}
	}
	return merged
}
