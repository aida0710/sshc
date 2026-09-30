package main

import (
	"slices"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// WireGuard のプロファイルの入力（sshc vpn add と edit）を読む。
//
// 設定ファイル（wg-quick の形）は鍵を含むので、OpenVPN の .ovpn と同じくシークレットとして
// 扱う。中身は []byte のまま運び、送ったあとに消す。設定ファイルのパスはターミナルから読み、
// 中身を確かめてから、Endpoint のサーバーと DNS を取り出す。

// wireGuardInput は、読み取った WireGuard の設定とシークレットである。
type wireGuardInput struct {
	settings *application.WireGuardProfile
	// resolvers は、設定ファイルの DNS である。プロファイルの DNS になる。
	resolvers []string
	secrets   []vpnSecretField
}

// readWireGuardProfile は、WireGuard の設定ファイルを読む。編集では、空欄なら保存済みの
// 設定ファイルのままにする。
//
// 保存済みの設定ファイルは見せない。CLI は、ほかのシークレット（sshc otp edit など）と同じく、
// 保存済みの値を取り出さずに入れ直させる。
func readWireGuardProfile(p vpnProfilePrompter, previous application.VPNProfile, keeps bool) (wireGuardInput, error) {
	label := "WireGuard configuration file (wg-quick format)"
	if keeps {
		label += " (blank keeps the saved file)"
	}
	path, err := promptVisibleSetup(p.ctx, p.stdin, p.prompt, label+": ", "")
	if err != nil {
		return wireGuardInput{}, err
	}
	if path == "" {
		if !keeps || previous.WireGuard == nil {
			return wireGuardInput{}, errVPNInputMissing
		}
		settings := &application.WireGuardProfile{Servers: previous.WireGuard.Servers}
		return wireGuardInput{settings: settings, resolvers: previous.DNS}, nil
	}
	text, err := readVPNInputFile(vpnInputFile{
		path: path, limit: vpn.MaxWireGuardConfigLength, description: "configuration file", kind: vpn.ErrSecrets,
		field: "secrets." + vpn.SecretKeyWireGuardConfig,
	})
	if err != nil {
		return wireGuardInput{}, err
	}
	config, err := vpn.ParseWireGuardConfig(text)
	if err != nil {
		clear(text)
		return wireGuardInput{}, vpnConfigInputError(err)
	}
	config.Forget()
	return wireGuardInput{
		settings:  &application.WireGuardProfile{Servers: config.Servers()},
		resolvers: slices.Clone(config.DNS),
		secrets:   []vpnSecretField{{name: vpn.SecretKeyWireGuardConfig, value: text}},
	}, nil
}
