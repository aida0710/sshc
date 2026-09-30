package main

import (
	"errors"
	"io"
	"os"
)

// 質問に答えてもらうコマンド（sshc sync setup、sshc vpn add、sshc vpn edit）が、
// ターミナルで動いているかを確かめる。

// errInteractivePromptRequired は、質問に答えるターミナルが無いことを表す。
var errInteractivePromptRequired = errors.New("an interactive terminal is required to answer the prompts")

// requireInteractivePrompt は、標準入力と質問を書く先がどちらもターミナルなら、
// 質問を書く先を返す。そうでなければ errInteractivePromptRequired を返す。失敗の文は
// 呼び出し側が自分の言い方で書く。
func requireInteractivePrompt(
	stdin *os.File, prompt io.Writer, terminal passwordTerminal,
) (*os.File, error) {
	promptFile, ok := prompt.(*os.File)
	if !ok || stdin == nil || terminal == nil ||
		!terminal.IsTerminal(int(stdin.Fd())) || !terminal.IsTerminal(int(promptFile.Fd())) {
		return nil, errInteractivePromptRequired
	}
	return promptFile, nil
}
