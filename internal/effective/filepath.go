package effective

import (
	"errors"
	"path/filepath"
	"strings"
)

// ファイル名の値を展開できない理由。
var (
	// ErrRelativePath は、作業ディレクトリに対して開かれる相対パスを断る。
	//
	// OpenSSH は相対の IdentityFile を ssh を起動した作業ディレクトリから開く。
	// sshc の接続にはその作業ディレクトリが無いので、ホームなどに読み替えて
	// 別のファイルを開くことはしない。
	ErrRelativePath = errors.New("a relative path is opened from the working directory of ssh")
	// ErrOtherUsersHome は、~user の形を断る。passwd を引かないと決まらない。
	ErrOtherUsersHome = errors.New("~user needs a password database lookup this resolver does not make")
)

// ExpandFilePath は、IdentityFile と UserKnownHostsFile の値を、接続のときに
// OpenSSH が開くパスへ展開する。
//
// OpenSSH の ssh.c と同じく、先頭の ~ をホームディレクトリに置き換えてから、${NAME} を
// 環境変数の値に、トークンをこの行き先の値に置き換える。展開できないトークンと値の
// 無い環境変数は断る。文字どおりのファイル名として使うと、存在しない鍵やファイルを
// 黙って探しにいく。
func ExpandFilePath(value string, facts LocalFacts, target TokenTarget) (string, error) {
	return expandFilePath(value, facts.Home, func(tilded string) (string, error) {
		return expandVariablesAndTokens(tilded, facts, target)
	})
}

// ExpandTildeFilePath は、先頭の ~ だけを展開する。
//
// GlobalKnownHostsFile の規則である。OpenSSH はこの値のトークンも ${NAME} も
// 展開せず、% や $ は文字どおりのファイル名になる。
func ExpandTildeFilePath(value, home string) (string, error) {
	return expandFilePath(value, home, func(tilded string) (string, error) { return tilded, nil })
}

// ExpandLocalFilePath は、接続先が決まる前に意味の定まる形だけを展開する。
//
// 展開するのは先頭の ~ と、%d と %% だけである。%h や %r を含む値は、どの接続先の
// 鍵かが決まらないので ErrUnknownToken で断る。${NAME} も、接続する側の環境で
// 値が変わるので断る。Keys の画面が、設定に書かれた鍵のパスを接続と同じ規則で
// 読むために使う。
func ExpandLocalFilePath(value, home string) (string, error) {
	if strings.Contains(value, variablePrefix) {
		return "", ErrUnknownToken
	}
	return expandFilePath(value, home, func(tilded string) (string, error) {
		if !usesOnlyTokens(tilded, "d") {
			return "", ErrUnknownToken
		}
		return ExpandTokens(tilded, LocalFacts{Home: home}, TokenTarget{})
	})
}

func expandFilePath(value, home string, expandTokens func(string) (string, error)) (string, error) {
	if value == "" {
		return "", ErrUnknownToken
	}
	tilded := value
	switch {
	case value == "~":
		tilded = home
	// Win32-OpenSSH は、Windows の区切りの ~\ も ~/ と同じくホームにする。
	case strings.HasPrefix(value, "~/"), strings.HasPrefix(value, "~"+string(filepath.Separator)):
		tilded = filepath.Join(home, value[2:])
	case strings.HasPrefix(value, "~"):
		return "", ErrOtherUsersHome
	}
	expanded, err := expandTokens(tilded)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(expanded) {
		return "", ErrRelativePath
	}
	return filepath.Clean(expanded), nil
}
