package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/application"
	"sshc/internal/vpn"
	"sshc/internal/vpnrefusal"
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
		if config, err = readOpenVPNConfigFile(path); err != nil {
			return nil, nil, err
		}
		summary, err := vpn.InspectOpenVPNConfig(config)
		if err != nil {
			zeroBytes(config)
			return nil, nil, openVPNConfigInputError(err)
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

// readOpenVPNConfigFile は、設定ファイルを上限の長さまで読む。先頭の ~/ はホーム
// ディレクトリとして読む。ターミナルの入力はシェルを通らないので、~ は展開されない。
func readOpenVPNConfigFile(path string) ([]byte, error) {
	if rest, found := strings.CutPrefix(path, "~/"); found {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, rest)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, &vpnInputError{
			sentence: fmt.Sprintf("設定ファイル「%s」を読み込めませんでした。%s",
				safeTerminalCell(path), safeTerminalCell(openVPNFileProblem(err))),
			cause: errVPNSetupInput,
		}
	}
	defer func() { _ = file.Close() }()
	// 上限より1バイト多く読み、上限を超えたかどうかを知る。
	config, err := io.ReadAll(io.LimitReader(file, vpn.MaxOpenVPNConfigLength+1))
	if err != nil {
		zeroBytes(config)
		return nil, err
	}
	if len(config) > vpn.MaxOpenVPNConfigLength {
		zeroBytes(config)
		return nil, openVPNConfigInputError(&vpn.FieldError{
			Kind: vpn.ErrSecrets, Field: "secrets." + vpn.SecretKeyOpenVPNConfig,
			Reason: vpn.ReasonTooLong, Limit: vpn.MaxOpenVPNConfigLength,
		})
	}
	return config, nil
}

// openVPNFileProblem は、設定ファイルを開けなかった理由を短く言う。
func openVPNFileProblem(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "ファイルが見つかりません。"
	case errors.Is(err, os.ErrPermission):
		return "ファイルを読む権限がありません。"
	}
	return err.Error()
}

// openVPNConfigInputError は、設定ファイルを断った理由を、engine が断ったときと同じ文にする。
func openVPNConfigInputError(err error) error {
	refusal, known := vpnrefusal.Of(err)
	if !known {
		return err
	}
	return &vpnInputError{sentence: vpnrefusal.Sentence(refusal), cause: errVPNSetupInput}
}
