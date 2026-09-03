package platform

import (
	"errors"
	"strings"
)

const LocalTerminalType = "xterm-256color"

// ErrNoLoginShell は、起動できるシェルがこのマシンに見つからないことを報告する。
var ErrNoLoginShell = errors.New("no login shell was found")
var ErrUnknownShellProfile = errors.New("local shell profile is not available")

// ShellProfile is a detected executable with fixed arguments. ID is persisted;
// command lines from HTTP or metadata are never interpreted by a shell.
type ShellProfile struct {
	ID        string
	Label     string
	Path      string
	Argv0     string
	Arguments []string
}

// ResolveShellProfile resolves a stored or one-shot ID only against profiles
// detected on this machine. An empty ID selects the machine's login shell.
func ResolveShellProfile(profiles []ShellProfile, id string) (ShellProfile, error) {
	if id == "" {
		id = "default"
	}
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return ShellProfile{}, ErrUnknownShellProfile
}

// シェルをどこから選び、どう起動するかは OS ごとに違う。同じ規則で書けるふりを
// しない。Unix のログインシェルは SHELL と `/bin` の下の絶対パスにあり、
// ハイフン付きの argv[0] で起きる。Windows にはそのどれも無く、実行してよいか
// どうかを実行ビットで測ることもできない。それぞれの規則は shell_unix.go と
// shell_windows.go にある。ここにあるのは、両方で同じであるものだけだ。

// LoginEnvironment は、ログインシェルへ渡してよい環境だけを残す。
//
// 端末は、それを起動したものの事情を継がない。開始ディレクトリと同じ話で
// ある。常駐プロセスの環境は、それを起動したものがたまたま持っていたもので
// あり、利用者はそのどれも選んでいない。npm run から起動されていれば、npm は
// 自分の設定を環境に詰めて渡してくる。`npm_config_prefix` は npm に渡した
// `--prefix` の写しであり、それを継いだシェルの中で nvm は「知らない prefix
// だ」と警告する。ここで開くのは利用者のシェルであって、ビルドの子ではない。
//
// npm由来の値は落とすだけでよい。起動するのはログインシェルなので、ユーザー本人が
// 設定した値はそのシェルが読む rc で再設定される。一方 TERM は、engineを
// 起動した端末ではなく、この関数の先にある埋込みTerminalの能力を表す値へ必ず置き換える。
//
// 落とすのは小文字の `npm_` だけである。npm が輸出するのはそれであり、
// NPM_TOKEN のような大文字はユーザーが自分で置いたものだからだ。
func LoginEnvironment(environ []string) []string {
	kept := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if found && (strings.HasPrefix(name, "npm_") || name == "INIT_CWD" || name == "NODE") {
			continue
		}
		// TERM describes the terminal emulator in front of the shell, not the
		// terminal (or launchd/systemd service) which happened to start sshc.
		// Inheriting an empty, dumb, or unrelated value makes line editors such
		// as zsh ZLE choose the wrong erase and cursor capabilities. SSH sessions
		// already advertise this same type when they request their remote PTY.
		if found && strings.EqualFold(name, "TERM") {
			continue
		}
		kept = append(kept, entry)
	}
	return append(kept, "TERM="+LocalTerminalType)
}
