package main

import (
	"io"
	"net/http"
	"os"
	"sshc/internal/app"
)

// commandEnvironment は、engine と話す短命な command（vault・sync・otp）が共有する
// 周辺: handoff の置き場、engine への HTTP client、標準入出力、password を
// 画面に出さずに読む端末。runner ごとに 5〜6 個の引数で回していたものを 1 つにする。
type commandEnvironment struct {
	// home は SSH 設定のワークスペース。connect と run だけが読む。
	home     string
	stateDir string
	client   *http.Client
	stdin    *os.File
	stdout   io.Writer
	stderr   io.Writer
	terminal passwordTerminal
}

func systemCommandEnvironment(home string, client *http.Client) commandEnvironment {
	return commandEnvironment{
		home: home, stateDir: app.HandoffDir(home), client: client,
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, terminal: systemPasswordTerminal{},
	}
}
