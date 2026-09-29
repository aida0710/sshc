package vpn

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

// ikev2 backend。strongSwan の charon を swanctl で操作し、IKEv2 でトンネルを張る。
//
// 経路は route-based にする。トンネルは XFRM インターフェース（if_id）に結び付け、
// strongSwan には経路を作らせない（install_routes = no）。サーバーが配るトラフィック
// セレクターが 0.0.0.0/0 でも、IPsec のポリシーはこのインターフェースを通る通信
// にしか効かない。接続先への経路は、ほかの backend と同じく connect が1つずつ
// このインターフェースへ作る。L2TP/IPsec の ipsec.conf（stroke）は if_id を
// 扱えないので、こちらは swanctl を使う。

// IKEv2Settings は、ikev2 backend の秘密でない設定である。
type IKEv2Settings struct {
	// Server は、VPNサーバーの名前またはアドレスである。名前はコンテナの中で
	// 名前解決する。
	Server string
	// Authentication は、こちらを認証する方式である（IKEv2AuthenticationEAP または
	// IKEv2AuthenticationPSK）。
	Authentication string
	// Identity は、こちらの ID である。EAP ではユーザー名として、IKE の ID と
	// EAP の ID の両方に使う。
	Identity string
	// ServerIdentity は、サーバーの ID である。空ならサーバーの名前（Server）を使う。
	ServerIdentity string
	// CACertificate は、サーバーの証明書を確かめる CA の証明書である（PEM）。空なら
	// 公的な認証局で確かめる。EAP のときだけ使う。PSK ではサーバーも事前共有鍵で
	// 認証する。
	CACertificate string
	// IKE と ESP は、サーバーに合わせて暗号スイートを指定する。L2TP/IPsec と同じ
	// 書き方で、空なら strongSwan の既定を使う。
	IKE string
	ESP string
}

// IKEv2 でこちらを認証する方式である。
const (
	// IKEv2AuthenticationEAP は、ユーザー名とパスワードで認証する（EAP-MSCHAPv2）。
	// Windows、macOS、スマートフォンの標準の IKEv2 VPN と同じ方式である。サーバーは
	// 証明書で認証する。
	IKEv2AuthenticationEAP = "eap-mschapv2"
	// IKEv2AuthenticationPSK は、事前共有鍵と ID で認証する。サーバーも同じ事前共有鍵で
	// 認証する。
	IKEv2AuthenticationPSK = "psk"
)

// IKEv2Secrets は、ikev2 backend の秘密である。使うのは、認証の方式に合う方だけである。
type IKEv2Secrets struct {
	// Password は、EAP-MSCHAPv2 のパスワードである。
	Password string
	// PreSharedKey は、事前共有鍵である。
	PreSharedKey string
}

const (
	// maxCACertificateLength は、CA の証明書（PEM）の長さの上限である。中間 CA を
	// 含めて数枚を並べても収まる大きさにする。API の IKEv2Profile と同じ値にする。
	maxCACertificateLength = 16384
	// pemCertificateType は、PEM のうち CA の証明書として受け付ける種類である。
	pemCertificateType = "CERTIFICATE"
)

// ikev2Link は、agent が作る XFRM インターフェースである。
//
// swanctl.conf の if_id と strongswan.conf の仮想 IP の置き場所が、このインター
// フェースを指す。値を一か所で決めて agent へも渡し、食い違わないようにする。
var ikev2Link = xfrmInterface{
	Name: "ipsec0",
	// ID は、IPsec のポリシーと SA をこのインターフェースに結び付ける if_id である。
	// 0 は「どのインターフェースにも結び付けない」を表すので使えない。コンテナの
	// 中にはこの経路しか無いので、ほかの値と重ならない。
	ID: 1,
	// MTU は、ESP を UDP で包むヘッダ（外側の IP、UDP、ESP、IV、パディング、ICV で
	// 100 バイトまで）を足しても 1500 に収まる大きさである。
	MTU: 1400,
}

// xfrmInterface は、XFRM インターフェースの名前と if_id と MTU である。
type xfrmInterface struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
	MTU  int    `json:"mtu"`
}

type ikev2Backend struct{}

// device は、渡すデバイスが無いことを返す。ESP はカーネルが処理し、XFRM
// インターフェースは NET_ADMIN で作れる。
func (ikev2Backend) device() string { return "" }

// capabilities は既定のままにする。charon がどの権限を使うかを、実際の VPN
// サーバーに対して確かめられていない。
func (ikev2Backend) capabilities() []string { return commonCapabilities }

func (ikev2Backend) validateSettings(profile Profile) error {
	settings := profile.IKEv2
	if settings == nil {
		return fieldError(ErrSettings, "ikev2", ReasonRequired)
	}
	if err := validateServerName("ikev2.server", settings.Server); err != nil {
		return err
	}
	switch settings.Authentication {
	case IKEv2AuthenticationEAP, IKEv2AuthenticationPSK:
	case "":
		return fieldError(ErrSettings, "ikev2.authentication", ReasonRequired)
	default:
		return fieldError(ErrSettings, "ikev2.authentication", ReasonUnsupported)
	}
	if err := validateUsername("ikev2.identity", settings.Identity); err != nil {
		return err
	}
	if err := validateServerIdentity(settings.ServerIdentity); err != nil {
		return err
	}
	if err := validateCACertificate(settings); err != nil {
		return err
	}
	return validateProposals("ikev2", settings.IKE, settings.ESP)
}

// validateServerIdentity は、サーバーの ID として swanctl.conf へ引用符で囲んで
// 書けるかを確かめる。空はサーバーの名前を使う。
//
// `%` で始まる値（`%any` など）は、strongSwan が「どの ID でもよい」などの特別な
// 意味に読む。サーバーの ID をそれにすると、信頼する認証局の証明書を持つ誰とでも
// 繋いでしまう。
func validateServerIdentity(identity string) error {
	const field = "ikev2.serverIdentity"
	if identity == "" {
		return nil
	}
	if err := validateLength(field, identity, maxUsernameLength); err != nil {
		return err
	}
	if strings.ContainsAny(identity, "\r\n\"\\") || strings.HasPrefix(identity, "%") {
		return fieldError(ErrSettings, field, ReasonFormat)
	}
	return nil
}

// validateCACertificate は、CA の証明書が PEM の証明書だけでできているかを確かめる。
// 秘密鍵などを貼り付けたまま保存させない。
func validateCACertificate(settings *IKEv2Settings) error {
	const field = "ikev2.caCertificate"
	if settings.CACertificate == "" {
		return nil
	}
	if settings.Authentication == IKEv2AuthenticationPSK {
		return fieldError(ErrSettings, field, ReasonUnexpected)
	}
	if err := validateLength(field, settings.CACertificate, maxCACertificateLength); err != nil {
		return err
	}
	if _, err := caCertificateBlocks(settings.CACertificate); err != nil {
		return fieldError(ErrSettings, field, ReasonFormat)
	}
	return nil
}

// caCertificateBlocks は、PEM の証明書を1枚ずつに分けて、PEM で書き直して返す。
//
// swanctl は1つのファイルから証明書を1枚しか読まないので、1枚ずつのファイルにする。
// 証明書のあいだと前後には空白しか置けない。pem.Decode は PEM の前の文字を読み
// 飛ばすので、それを自分で断る。
func caCertificateBlocks(text string) ([]string, error) {
	var certificates []string
	rest := bytes.TrimSpace([]byte(text))
	for len(rest) > 0 {
		if !bytes.HasPrefix(rest, []byte("-----BEGIN ")) {
			return nil, fmt.Errorf("text outside a PEM block")
		}
		block, remaining := pem.Decode(rest)
		if block == nil || block.Type != pemCertificateType || len(block.Headers) != 0 {
			return nil, fmt.Errorf("not a PEM certificate")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, err
		}
		certificates = append(certificates, string(pem.EncodeToMemory(block)))
		rest = bytes.TrimSpace(remaining)
	}
	if len(certificates) == 0 {
		return nil, fmt.Errorf("no certificate")
	}
	return certificates, nil
}

func (ikev2Backend) validateSecrets(profile Profile, secrets Secrets) error {
	var stored IKEv2Secrets
	if secrets.IKEv2 != nil {
		stored = *secrets.IKEv2
	}
	if profile.IKEv2 != nil && profile.IKEv2.Authentication == IKEv2AuthenticationPSK {
		return requireSecret("secrets."+SecretKeyIKEv2PSK, stored.PreSharedKey, maxSecretLength)
	}
	return requireSecret("secrets."+SecretKeyIKEv2Password, stored.Password, maxSecretLength)
}

func (ikev2Backend) writeAgentSection(request agentSectionRequest, document *agentDocument) error {
	settings := *request.profile.IKEv2
	documents, err := ikev2Documents(settings, *request.secrets.IKEv2)
	if err != nil {
		return err
	}
	document.IKEv2 = &ikev2Document{
		strongSwanDocument: strongSwanDocument{Server: settings.Server, Documents: documents},
		Link:               ikev2Link,
		PublicAuthorities:  settings.Authentication == IKEv2AuthenticationEAP && settings.CACertificate == "",
	}
	return nil
}

// secretValues は、パスワードと事前共有鍵、その16進表記を返す。どちらも設定へ16進で
// 書くので、その形のままログに現れうる。
func (ikev2Backend) secretValues(secrets Secrets) []string {
	if secrets.IKEv2 == nil {
		return nil
	}
	return append(secretAndHexForms(secrets.IKEv2.Password), secretAndHexForms(secrets.IKEv2.PreSharedKey)...)
}

func (ikev2Backend) waitsForApproval(Profile) bool { return false }

func (ikev2Backend) ownSecrets(secrets Secrets) Secrets { return Secrets{IKEv2: secrets.IKEv2} }
