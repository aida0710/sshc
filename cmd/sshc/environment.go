package main

import (
	"io"
	"net/http"
	"os"
	"sshc/internal/app"
)

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

func systemCommandEnvironment(home string, client *http.Client) commandEnvironment {
	return commandEnvironment{
		home: home, stateDir: app.HandoffDir(home), client: client,
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, terminal: systemPasswordTerminal{},
		confirm: systemActionConfirmer,
	}
}
