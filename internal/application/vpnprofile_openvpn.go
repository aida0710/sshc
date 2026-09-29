package application

import "sshc/internal/vpn"

// OpenVPNProfile は、openvpn backend のシークレットでない設定である。
//
// 設定ファイル（.ovpn）は鍵を含むことが多いので Vault に置く。ここに持つのは、設定
// ファイルの remote から取り出したサーバーと、auth-user-pass で送るユーザー名だけである。
type OpenVPNProfile struct {
	// Servers は、設定ファイルの remote のサーバーである。一覧の表示に使う。保存する
	// ときに、Vault の設定ファイルと合うことを確かめる。
	Servers []string `json:"servers"`
	// Username は、auth-user-pass で送るユーザー名である。空なら送らない。パスワードは
	// Vault にある。
	Username string `json:"username,omitempty"`
}

// openVPNSettings は、保存形式を internal/vpn の設定へ直す。
func (stored OpenVPNProfile) openVPNSettings() *vpn.OpenVPNSettings {
	return &vpn.OpenVPNSettings{
		Servers: append([]string(nil), stored.Servers...), Username: stored.Username,
	}
}
