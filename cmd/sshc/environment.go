package main

import (
	"io"
	"net/http"
	"os"
)

// userPaths は、1 回の呼び出しが使う利用者の home と、そこから解決した sshc の
// state directory である。state directory は app.StateDir で一度だけ解決し、
// engine lock・handoff の読み書きがどれも同じ答えを使うようにする。
type userPaths struct {
	home     string
	stateDir string
}

// commandEnvironment は、engine と話す短命な command が共有する周辺である:
// handoff の置き場、engine への HTTP client、標準入出力、password を画面に出さずに
// 読む端末、変更を進めてよいかの確認。
type commandEnvironment struct {
	// home は SSH 設定のワークスペース。connect と run だけが読む。
	home     string
	stateDir string
	client   *http.Client
	stdin    *os.File
	stdout   io.Writer
	stderr   io.Writer
	terminal passwordTerminal
	// confirm は、変更を進めてよいかを利用者に尋ねる。sftp だけが使う。
	confirm actionConfirmer
}

func systemCommandEnvironment(paths userPaths, client *http.Client) commandEnvironment {
	return commandEnvironment{
		home: paths.home, stateDir: paths.stateDir, client: client,
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, terminal: systemPasswordTerminal{},
		confirm: systemActionConfirmer,
	}
}
