package vpn

import (
	"encoding/json"
	"fmt"
)

// 秘密は、プロファイルひとつにつき一件の記録として Vault に置く。秘密は同時に
// 作られ、同時に回し、同時に消えるので、backend ごとに分けて持つと、プロファイルを
// 作る・改名する・消すたびに複数件の整合を取ることになる。
//
// 記録の JSON のキーは v0.38.0 から変えていない。Vault は端末のあいだで同期され、
// 古い版が同じ記録を読む。Go の型との対応は、この file の Encode と Decode に閉じる。

// Secrets は、プロファイルの秘密である。Vaultから読み、標準入力でコンテナへ渡す。
//
// 使うのは、そのプロファイルの backend の節だけである。
type Secrets struct {
	WireGuard   *WireGuardSecrets
	L2TP        *L2TPSecrets
	OpenConnect *OpenConnectSecrets
}

// WireGuardSecrets は、wireguard backend の秘密である。
type WireGuardSecrets struct {
	PrivateKey string
}

// L2TPSecrets は、l2tp_ipsec backend の秘密である。
type L2TPSecrets struct {
	// Password は、VPNの利用者のパスワードである。
	Password string
	// PreSharedKey は、IPsecの事前共有鍵である。
	PreSharedKey string
}

// OpenConnectSecrets は、openconnect backend の秘密である。
type OpenConnectSecrets struct {
	Password string
	// TOTPSecret は、二段目のコードを作る種である（base32 または otpauth URI）。
	// SecondFactor が totp のときだけ使う。
	TOTPSecret string
}

// 記録と API の本文で使う、秘密ひとつずつの JSON のキーである。CLI は秘密を Go の
// 文字列にせずに本文を組み立てるので、型ではなくキーの名前を共有する。
const (
	SecretKeyWireGuardPrivateKey   = "wireguardPrivateKey"
	SecretKeyL2TPPassword          = "l2tpPassword"
	SecretKeyIPsecPSK              = "ipsecPsk"
	SecretKeyOpenConnectPassword   = "openconnectPassword"
	SecretKeyOpenConnectTOTPSecret = "openconnectTotpSecret"
)

// SecretsDocument は、秘密の JSON の形である。Vault の記録と、保存要求の本文が
// この形を使う。空の項目は「値が無い」を表す。
type SecretsDocument struct {
	WireGuardPrivateKey   string `json:"wireguardPrivateKey,omitempty"`
	L2TPPassword          string `json:"l2tpPassword,omitempty"`
	IPsecPSK              string `json:"ipsecPsk,omitempty"`
	OpenConnectPassword   string `json:"openconnectPassword,omitempty"`
	OpenConnectTOTPSecret string `json:"openconnectTotpSecret,omitempty"`
}

// Secrets は、JSON の形を backend ごとの型へ直す。値のある backend の節だけを作る。
func (document SecretsDocument) Secrets() Secrets {
	var secrets Secrets
	if document.WireGuardPrivateKey != "" {
		secrets.WireGuard = &WireGuardSecrets{PrivateKey: document.WireGuardPrivateKey}
	}
	if document.L2TPPassword != "" || document.IPsecPSK != "" {
		secrets.L2TP = &L2TPSecrets{Password: document.L2TPPassword, PreSharedKey: document.IPsecPSK}
	}
	if document.OpenConnectPassword != "" || document.OpenConnectTOTPSecret != "" {
		secrets.OpenConnect = &OpenConnectSecrets{
			Password: document.OpenConnectPassword, TOTPSecret: document.OpenConnectTOTPSecret,
		}
	}
	return secrets
}

// Document は、backend ごとの型を JSON の形へ直す。
func (secrets Secrets) Document() SecretsDocument {
	var document SecretsDocument
	if secrets.WireGuard != nil {
		document.WireGuardPrivateKey = secrets.WireGuard.PrivateKey
	}
	if secrets.L2TP != nil {
		document.L2TPPassword = secrets.L2TP.Password
		document.IPsecPSK = secrets.L2TP.PreSharedKey
	}
	if secrets.OpenConnect != nil {
		document.OpenConnectPassword = secrets.OpenConnect.Password
		document.OpenConnectTOTPSecret = secrets.OpenConnect.TOTPSecret
	}
	return document
}

// EncodeSecrets は、Vault へ保存する一件ぶんの記録を作る。
func EncodeSecrets(secrets Secrets) (string, error) {
	encoded, err := json.Marshal(secrets.Document())
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// DecodeSecrets は、Vault から読んだ記録を戻す。
func DecodeSecrets(stored string) (Secrets, error) {
	var document SecretsDocument
	if err := json.Unmarshal([]byte(stored), &document); err != nil {
		return Secrets{}, fmt.Errorf("%w: %w", ErrSecrets, err)
	}
	return document.Secrets(), nil
}
