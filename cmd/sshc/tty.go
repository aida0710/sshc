package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// controllingTerminal は /dev/tty を通して人間と話す。
//
// 標準入力ではなく /dev/tty を開くのは、OpenSSH がヘルパーの標準入力をパイプに
// しているからである。制御端末はそれとは別に存在し、そこに打ち手がいる。
//
// このファイルだけは単体テストが触れない。本物の端末を開くテストは、それを走らせて
// いる人の端末を入力待ちで止めてしまう。テストは terminalPrompter を差し替える方を
// 試し、ここが試すのは「/dev/tty を開く」という一点だけである。
type controllingTerminal struct{ tty *os.File }

// openControllingTerminal は制御端末を開く。
//
// 開けないのは、答える人間がいないということである。systemd や launchd から
// -open=false で上がったエージェントがそれにあたる。
func openControllingTerminal() (terminalPrompter, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return controllingTerminal{tty: tty}, nil
}

// Prompt は question を端末に出し、打たれた 1 行を返す。
//
// echo が偽のとき入力を伏せるのは、その行がパスフレーズやワンタイムコードで
// ありうるからである。伏せると打ち手の Enter も表示されないので、行を閉じるのは
// こちらの仕事になる。
func (t controllingTerminal) Prompt(question string, echo bool) (string, error) {
	if _, err := io.WriteString(t.tty, question); err != nil {
		return "", err
	}
	if !echo {
		typed, err := term.ReadPassword(int(t.tty.Fd()))
		_, _ = io.WriteString(t.tty, "\n")
		return string(typed), err
	}
	return readEchoedLine(t.tty)
}

// readEchoedLine は、伏せずに読む回答を 1 行受け取る。
func readEchoedLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	typed := strings.TrimRight(line, "\r\n")
	// 端末が閉じられた場合、最後の行は改行なしで届く。読めた分は答えとして扱う。
	// ただし何も打たれないまま閉じたのなら、それは回答ではない。空文字を返せば
	// ssh には「yes ではない」と伝わって接続は止まるが、なぜ止まったのかが誰にも
	// 分からなくなる。読めなかったことは読めなかったと言う。
	if typed == "" && errors.Is(err, io.EOF) {
		return "", io.ErrUnexpectedEOF
	}
	return typed, nil
}

func (t controllingTerminal) Close() error { return t.tty.Close() }
