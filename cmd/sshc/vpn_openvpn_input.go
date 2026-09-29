package main

import (
	"sshc/internal/application"
	"sshc/internal/vpn"
)

// OpenVPN のプロファイルの入力（sshc vpn add と edit）を読む。
//
// 設定ファイル（.ovpn）は鍵を含むことが多いのでシークレットとして扱う。中身は []byte の
// まま運び、送ったあとに消す。設定ファイルのパスはターミナルから読み、中身を確かめてから
// remote のサーバーを取り出す。

// readOpenVPNProfile は、OpenVPN の設定ファイルと、任意のユーザー名とパスワードを読む。
func readOpenVPNProfile(
	p vpnProfilePrompter, current *application.OpenVPNProfile, keeps bool,
) (*application.OpenVPNProfile, []vpnSecretField, error) {
	var previous application.OpenVPNProfile
	if current != nil {
		previous = *current
	}
	label := "OpenVPN configuration file (.ovpn)"
	if keeps {
		label += " (blank keeps the saved file)"
	}
	path, err := promptVisibleSetup(p.ctx, p.stdin, p.prompt, label+": ", "")
	if err != nil {
		return nil, nil, err
	}
	settings := &application.OpenVPNProfile{Servers: previous.Servers}
	var config []byte
	if path != "" {
		if config, err = readVPNConfigFile(vpnConfigFile{
			path: path, limit: vpn.MaxOpenVPNConfigLength, kind: vpn.ErrSecrets,
			field: "secrets." + vpn.SecretKeyOpenVPNConfig,
		}); err != nil {
			return nil, nil, err
		}
		summary, err := vpn.InspectOpenVPNConfig(config)
		if err != nil {
			zeroBytes(config)
			return nil, nil, vpnConfigInputError(err)
		}
		settings.Servers = summary.Servers
	} else if !keeps {
		return nil, nil, errVPNInputMissing
	}
	// 設定ファイルが auth-user-pass を含むなら、ユーザー名とパスワードが要る。含まなければ
	// 空のままでよい（証明書だけで認証する）。
	if settings.Username, err = p.optional("VPN username", previous.Username); err != nil {
		zeroBytes(config)
		return nil, nil, err
	}
	fields := []vpnSecretField{{name: vpn.SecretKeyOpenVPNConfig, value: config}}
	if settings.Username != "" {
		// 保存済みのパスワードがあるのは、前もユーザー名を使っていた場合だけである。
		keepsPassword := keeps && previous.Username != ""
		password, err := p.secret("VPN password", keepsPassword)
		if err != nil {
			zeroBytes(config)
			return nil, nil, err
		}
		fields = append(fields, vpnSecretField{name: vpn.SecretKeyOpenVPNPassword, value: password})
		if err := requireSecret(password, keepsPassword); err != nil {
			return nil, sentSecrets(fields...), err
		}
	}
	return settings, sentSecrets(fields...), nil
}
