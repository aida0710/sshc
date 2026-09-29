package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/vpn"
	"sshc/internal/vpnrefusal"
)

// VPN プロファイルの設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）を、
// ターミナルで入力されたパスから読む。

// vpnConfigFile は、読む設定ファイルと、その上限と、断るときに名指す項目である。
type vpnConfigFile struct {
	path  string
	limit int
	// kind と field は、長すぎるときに返す項目の誤りの分類と JSON パスである。
	kind  error
	field string
}

// readVPNConfigFile は、設定ファイルを上限の長さまで読む。先頭の ~/ はホーム
// ディレクトリとして読む。ターミナルの入力はシェルを通らないので、~ は展開されない。
//
// 設定ファイルは鍵を含みうるので []byte で返す。呼び出し側が使い終わったら消す。
func readVPNConfigFile(wanted vpnConfigFile) ([]byte, error) {
	path := wanted.path
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
				safeTerminalCell(path), safeTerminalCell(vpnConfigFileProblem(err))),
			cause: errVPNSetupInput,
		}
	}
	defer func() { _ = file.Close() }()
	// 上限より1バイト多く読み、上限を超えたかどうかを知る。
	config, err := io.ReadAll(io.LimitReader(file, int64(wanted.limit)+1))
	if err != nil {
		zeroBytes(config)
		return nil, err
	}
	if len(config) > wanted.limit {
		zeroBytes(config)
		return nil, vpnConfigInputError(&vpn.FieldError{
			Kind: wanted.kind, Field: wanted.field, Reason: vpn.ReasonTooLong, Limit: wanted.limit,
		})
	}
	return config, nil
}

// vpnConfigFileProblem は、設定ファイルを開けなかった理由を短く言う。
func vpnConfigFileProblem(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "ファイルが見つかりません。"
	case errors.Is(err, os.ErrPermission):
		return "ファイルを読む権限がありません。"
	}
	return err.Error()
}

// vpnConfigInputError は、設定ファイルを断った理由を、engine が断ったときと同じ文にする。
func vpnConfigInputError(err error) error {
	refusal, known := vpnrefusal.Of(err)
	if !known {
		return err
	}
	return &vpnInputError{sentence: vpnrefusal.Sentence(refusal), cause: errVPNSetupInput}
}
