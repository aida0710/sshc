package vpn

import (
	"encoding/json"
	"fmt"
)

// 秘密は、プロファイルひとつにつき一件の記録として Vault に置く。秘密は同時に
// 作られ、同時に回し、同時に消えるので、backend ごとに分けて持つと、プロファイルを
// 作る・改名する・消すたびに複数件の整合を取ることになる。

type secretsDocument struct {
	WireGuardPrivateKey string `json:"wireguardPrivateKey,omitempty"`
	L2TPPassword        string `json:"l2tpPassword,omitempty"`
	IPsecPSK            string `json:"ipsecPsk,omitempty"`
	OpenConnectPassword string `json:"openconnectPassword,omitempty"`
	// OpenConnectTOTPSecret は、二段目のコードを作る種である。
	OpenConnectTOTPSecret string `json:"openconnectTotpSecret,omitempty"`
}

// EncodeSecrets は、Vault へ保存する一件ぶんの記録を作る。
func EncodeSecrets(secrets Secrets) (string, error) {
	encoded, err := json.Marshal(secretsDocument{
		WireGuardPrivateKey:   secrets.WireGuardPrivateKey,
		L2TPPassword:          secrets.L2TPPassword,
		IPsecPSK:              secrets.IPsecPSK,
		OpenConnectPassword:   secrets.OpenConnectPassword,
		OpenConnectTOTPSecret: secrets.OpenConnectTOTPSecret,
	})
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// DecodeSecrets は、Vault から読んだ記録を戻す。
func DecodeSecrets(stored string) (Secrets, error) {
	var document secretsDocument
	if err := json.Unmarshal([]byte(stored), &document); err != nil {
		return Secrets{}, fmt.Errorf("%w: %w", ErrSecrets, err)
	}
	return Secrets{
		WireGuardPrivateKey:   document.WireGuardPrivateKey,
		L2TPPassword:          document.L2TPPassword,
		IPsecPSK:              document.IPsecPSK,
		OpenConnectPassword:   document.OpenConnectPassword,
		OpenConnectTOTPSecret: document.OpenConnectTOTPSecret,
	}, nil
}
