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

// VPN プロファイルの入力で、中身をファイルから受け取る項目（OpenVPN の .ovpn、
// WireGuard の設定ファイル、IKEv2 の CA の証明書）を、ターミナルで入力されたパスから読む。

// vpnInputFile は、読むファイルと、その上限と、断るときに名指すものである。
type vpnInputFile struct {
	path  string
	limit int
	// description は、読めないときの文でファイルを呼ぶ名前である（"configuration file" など）。
	description string
	// kind と field は、長すぎるときに返す項目の誤りの分類と JSON パスである。engine が
	// 同じ値を断るときと同じ文にする。
	kind  error
	field string
}

// readVPNInputFile は、ファイルを上限の長さまで読む。先頭の ~/ はホーム
// ディレクトリとして読む。ターミナルの入力はシェルを通らないので、~ は展開されない。
//
// 設定ファイルは鍵を含みうるので []byte で返す。呼び出し側が使い終わったら消す。
func readVPNInputFile(wanted vpnInputFile) ([]byte, error) {
	path := wanted.path
	if rest, found := strings.CutPrefix(path, "~/"); found {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, unreadableVPNInputFile(wanted, path, err)
		}
		path = filepath.Join(home, rest)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, unreadableVPNInputFile(wanted, path, err)
	}
	defer func() { _ = file.Close() }()
	// ディレクトリは開けても読めない。読んだときの誤りは OS ごとに文が違うので、先に見分ける。
	if info, err := file.Stat(); err == nil && info.IsDir() {
		return nil, unreadableVPNInputFile(wanted, path, errVPNInputFileIsDirectory)
	}
	// 上限より1バイト多く読み、上限を超えたかどうかを知る。
	config, err := io.ReadAll(io.LimitReader(file, int64(wanted.limit)+1))
	if err != nil {
		clear(config)
		return nil, unreadableVPNInputFile(wanted, path, err)
	}
	if len(config) > wanted.limit {
		clear(config)
		return nil, vpnConfigInputError(&vpn.FieldError{
			Kind: wanted.kind, Field: wanted.field, Reason: vpn.ReasonTooLong, Limit: wanted.limit,
		})
	}
	return config, nil
}

// errVPNInputFileIsDirectory は、ファイルの代わりにディレクトリのパスが入力されたことを表す。
var errVPNInputFileIsDirectory = errors.New("the path is a directory")

// unreadableVPNInputFile は、ファイルを読めなかったことを、どのファイルがなぜ読めないかを
// 添えた入力の誤りにする。engine に届く前の誤りなので、engine の不調として伝えない。
func unreadableVPNInputFile(wanted vpnInputFile, path string, err error) error {
	return &vpnInputError{
		sentence: fmt.Sprintf("The %s \"%s\" could not be read. %s", wanted.description,
			safeTerminalCell(path), safeTerminalCell(vpnInputFileProblem(err))),
		cause: errVPNSetupInput,
	}
}

// vpnInputFileProblem は、ファイルを読めなかった理由を短く言う。
func vpnInputFileProblem(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "The file does not exist."
	case errors.Is(err, os.ErrPermission):
		return "You do not have permission to read the file."
	case errors.Is(err, errVPNInputFileIsDirectory):
		return "The path is a directory, not a file."
	}
	return err.Error()
}

// vpnConfigInputError は、ファイルの中身を断った理由を、engine が断ったときと同じ文にする。
func vpnConfigInputError(err error) error {
	refusal, known := vpnrefusal.Of(err)
	if !known {
		return err
	}
	return &vpnInputError{sentence: vpnrefusal.EnglishSentence(refusal), cause: errVPNSetupInput}
}
