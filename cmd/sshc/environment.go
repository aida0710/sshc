package main

import (
	"io"
	"net/http"
	"os"
)

// commandEnvironment は、engine と話す短命な command（vault・sync・otp）が共有する
// 周辺: handoff の置き場、engine への HTTP client、標準入出力、password を
// 画面に出さずに読む端末。runner ごとに 5〜6 個の引数で回していたものを 1 つにする。
type commandEnvironment struct {
	stateDir string
	client   *http.Client
	stdin    *os.File
	stdout   io.Writer
	stderr   io.Writer
	terminal passwordTerminal
}

func systemCommandEnvironment(stateDir string, client *http.Client) commandEnvironment {
	return commandEnvironment{
		stateDir: stateDir, client: client,
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, terminal: systemPasswordTerminal{},
	}
}
