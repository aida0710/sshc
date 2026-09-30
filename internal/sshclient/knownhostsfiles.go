package sshclient

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/effective"
	"sshc/internal/sshmatch"
)

// ErrKnownHostsFile は、UserKnownHostsFile か GlobalKnownHostsFile のファイル名を
// 決められなかったことを報告する。
//
// 読み飛ばして繋ぐことはしない。そのファイルにある鍵と照合できないまま、未知の
// ホストとして鍵を受け入れることになるからである。断るのはホスト鍵を照合する
// ときだけで、Target の組み立て（認証の束縛の計算や設定の保存）は止めない。
var ErrKnownHostsFile = errors.New("a known_hosts file name cannot be expanded")

// KnownHostsFiles は、ホスト鍵の照合に使う known_hosts のファイルである。
//
// OpenSSH と同じく、照合は User と Global のすべてのファイルで行い、受け入れた
// 鍵は User の最初のファイルへ書く。
type KnownHostsFiles struct {
	// User は UserKnownHostsFile。既定は ~/.ssh/known_hosts と ~/.ssh/known_hosts2。
	User []string
	// Global は GlobalKnownHostsFile。照合にだけ使う。組織が配るホスト鍵はここにある。
	Global []string
	// ExpansionError は、ファイル名を決められなかった理由（ErrKnownHostsFile）である。
	// nil でなければ、ホスト鍵の照合はこれを返して接続を断る。
	ExpansionError error
}

// all は、照合に使うファイルを OpenSSH が読む順（User、Global）に返す。
func (files KnownHostsFiles) all() []string {
	return append(append([]string(nil), files.User...), files.Global...)
}

// defaultUserKnownHostsFiles は、UserKnownHostsFile が書かれていないときのファイルである。
func defaultUserKnownHostsFiles(home string) []string {
	return []string{
		filepath.Join(home, ".ssh", "known_hosts"),
		filepath.Join(home, ".ssh", "known_hosts2"),
	}
}

// unixNullDevice は、設定に書かれる null デバイスの名前である。
const unixNullDevice = "/dev/null"

// knownHostsFilesFrom は、設定の UserKnownHostsFile と GlobalKnownHostsFile を
// ファイルの絶対パスへ展開する。"none" はファイルを使わないという意味である。
//
// OpenSSH と同じく、UserKnownHostsFile は ~、${NAME}、トークンを展開し、
// GlobalKnownHostsFile は ~ だけを展開する。
func knownHostsFilesFrom(values effective.Values, facts effective.LocalFacts, tokens effective.TokenTarget) KnownHostsFiles {
	user, err := knownHostsPaths(values.All("userknownhostsfile"), defaultUserKnownHostsFiles(facts.Home), func(entry string) (string, error) {
		return effective.ExpandFilePath(entry, facts, tokens)
	})
	if err != nil {
		return KnownHostsFiles{ExpansionError: err}
	}
	global, err := knownHostsPaths(values.All("globalknownhostsfile"), facts.GlobalKnownHostsFiles, func(entry string) (string, error) {
		return effective.ExpandTildeFilePath(entry, facts.Home)
	})
	if err != nil {
		return KnownHostsFiles{ExpansionError: err}
	}
	return KnownHostsFiles{User: user, Global: global}
}

func knownHostsPaths(written, defaults []string, expand func(string) (string, error)) ([]string, error) {
	if len(written) == 0 {
		return defaults, nil
	}
	if len(written) == 1 && strings.EqualFold(written[0], "none") {
		return nil, nil
	}
	paths := make([]string, 0, len(written))
	for _, entry := range written {
		// 使い捨てのホストに繋ぐ設定の `UserKnownHostsFile /dev/null` は、OS を
		// 問わず書かれる。Windows 版の OpenSSH と同じく、この OS の null デバイスに読み替える。
		if entry == unixNullDevice {
			paths = append(paths, os.DevNull)
			continue
		}
		path, err := expand(entry)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrKnownHostsFile, entry, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// jumpHostToken は %j の値である。OpenSSH は ProxyJump の最後のホップのホスト名を使う。
func jumpHostToken(chain effective.Chain) string {
	if chain.Disabled || len(chain.Hops) == 0 {
		return ""
	}
	return chain.Hops[len(chain.Hops)-1].Host
}

// hostKeyAliasFrom は HostKeyAlias を読む。OpenSSH は ssh.c でこれを小文字にする。
func hostKeyAliasFrom(values effective.Values) string {
	return sshmatch.LowerASCII(values.First("hostkeyalias"))
}
