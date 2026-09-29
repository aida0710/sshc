package application

import "sshc/internal/vpn"

// IKEv2Profile は、ikev2 backend のシークレットでない設定である。パスワードと事前
// 共有鍵は Vault にある。
type IKEv2Profile struct {
	// Server は、VPNサーバーの名前またはアドレスである。名前はコンテナの中で名前解決する。
	Server string `json:"server"`
	// Authentication は、こちらを認証する方式である（`eap-mschapv2` または `psk`）。
	Authentication string `json:"authentication"`
	// Identity は、こちらの ID である。EAP ではユーザー名である。
	Identity string `json:"identity"`
	// ServerIdentity は、サーバーの ID である。空ならサーバーの名前を使う。
	ServerIdentity string `json:"serverIdentity,omitempty"`
	// CACertificate は、サーバーの証明書を確かめる CA の証明書（PEM）である。空なら
	// 公的な認証局で確かめる。
	CACertificate string `json:"caCertificate,omitempty"`
	// IKE と ESP は、サーバーと暗号スイートが合わないときだけ書く。
	IKE string `json:"ike,omitempty"`
	ESP string `json:"esp,omitempty"`
}

// ikev2Settings は、保存形式を internal/vpn の設定へ直す。
func (stored IKEv2Profile) ikev2Settings() *vpn.IKEv2Settings {
	return &vpn.IKEv2Settings{
		Server:         stored.Server,
		Authentication: stored.Authentication,
		Identity:       stored.Identity,
		ServerIdentity: stored.ServerIdentity,
		CACertificate:  stored.CACertificate,
		IKE:            stored.IKE,
		ESP:            stored.ESP,
	}
}
