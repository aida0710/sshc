// Package telnettest は、ほかの package のテストが仮想の Telnet サーバを書くときに使う、
// 受け入れた接続のサーバ側である。
//
// Telnet では、オプションの交渉と本文の順番が決まっていない。クライアントはサーバの
// WILL を読んだときに初めて返事を送るので、WILL が遅れて届けば、送信文が返事より先に
// 届く。ServerConn は、本物のサーバと同じく、届いた順番を問わずに読む。
package telnettest

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"

	"sshc/internal/iowrite"
)

// RFC 854 と RFC 857 の命令とオプションのうち、仮想サーバが読み書きするもの。
const (
	commandSB   byte = 250
	commandWILL byte = 251
	commandWONT byte = 252
	commandDO   byte = 253
	commandDONT byte = 254
	commandIAC  byte = 255
	optionEcho  byte = 1
)

// maxLineBytes は、ReadLine が1行として受け取る上限。テストの送信文は短いので、CR LF を
// 送らないクライアントがいても、サーバが際限なく溜めないようにする。
const maxLineBytes = 1 << 10

// lineEnding は、Telnet の NVT が1行の終わりに送る CR LF。
var lineEnding = []byte("\r\n")

// ServerConn は、仮想の Telnet サーバが受け入れた接続1本のサーバ側である。読むのは
// 1つの goroutine に限る。応答は元の net.Conn へ直接書いてよい。
type ServerConn struct {
	connection   net.Conn
	reader       *bufio.Reader
	echoAccepted bool
}

// NewServerConn は、受け入れた接続を Telnet として読む ServerConn を作る。
func NewServerConn(connection net.Conn) *ServerConn {
	return &ServerConn{connection: connection, reader: bufio.NewReader(connection)}
}

// OfferEcho は、サーバが文字を送り返す（IAC WILL ECHO）と申し出る。
func (server *ServerConn) OfferEcho() error {
	return iowrite.WriteAll(server.connection, []byte{commandIAC, commandWILL, optionEcho})
}

// ReadLineAfterOfferingEcho は、ECHO を申し出てから送信文を1行読み、申し出が受けられた
// ことを確かめる。クライアントの返事と送信文は、どちらが先に届いてもよい。
func (server *ServerConn) ReadLineAfterOfferingEcho() (string, error) {
	if err := server.OfferEcho(); err != nil {
		return "", err
	}
	line, err := server.ReadLine()
	if err != nil {
		return "", err
	}
	return line, server.AwaitEchoAccepted()
}

// ReadLine は、クライアントの本文を CR LF まで読み、CR LF を含めて返す。途中に届いた
// 交渉の返事は覚えておき、AwaitEchoAccepted で確かめる。
func (server *ServerConn) ReadLine() (string, error) {
	line := make([]byte, 0, len(lineEnding))
	for len(line) < maxLineBytes {
		value, isData, err := server.readByte()
		if err != nil {
			return "", err
		}
		if !isData {
			continue
		}
		line = append(line, value)
		if bytes.HasSuffix(line, lineEnding) {
			return string(line), nil
		}
	}
	return "", fmt.Errorf("telnettest: line exceeded %d bytes", maxLineBytes)
}

// AwaitEchoAccepted は、クライアントが IAC DO ECHO で申し出を受けるまで読む。ReadLine の
// 途中で届いていれば、すぐに返る。返事より前に本文が届いたら失敗にする。
func (server *ServerConn) AwaitEchoAccepted() error {
	for !server.echoAccepted {
		value, isData, err := server.readByte()
		if err != nil {
			return err
		}
		if isData {
			return fmt.Errorf("telnettest: application byte %d arrived before IAC DO ECHO", value)
		}
	}
	return nil
}

// DrainUntilClosed は、クライアントが接続を閉じるまで読み捨てる。読んでいないバイトを
// 残したままサーバが先に閉じると RST になり、Windows ではクライアントがまだ読んでいない
// 応答まで捨てられる。
func (server *ServerConn) DrainUntilClosed() error {
	_, err := io.Copy(io.Discard, server.reader)
	return err
}

// readByte は、クライアントのバイトを1つ読む。IAC で始まる命令は最後まで読み、本文の
// バイトではないこと（isData が false）を返す。
func (server *ServerConn) readByte() (value byte, isData bool, err error) {
	value, err = server.reader.ReadByte()
	if err != nil {
		return 0, false, err
	}
	if value != commandIAC {
		return value, true, nil
	}
	command, err := server.reader.ReadByte()
	if err != nil {
		return 0, false, err
	}
	switch command {
	case commandIAC:
		return commandIAC, true, nil
	case commandWILL, commandWONT, commandDO, commandDONT:
		option, err := server.reader.ReadByte()
		if err != nil {
			return 0, false, err
		}
		return 0, false, server.recordNegotiation(command, option)
	case commandSB:
		return 0, false, errors.New("telnettest: the client started a subnegotiation that this server never requested")
	default:
		// NOP や GA などの2バイトの命令は、本文を持たない。
		return 0, false, nil
	}
}

// recordNegotiation は、クライアントの交渉の返事を覚える。仮想サーバが申し出るのは
// ECHO だけなので、それ以外の返事は失敗にする。
func (server *ServerConn) recordNegotiation(command, option byte) error {
	if command != commandDO || option != optionEcho {
		return fmt.Errorf("telnettest: unexpected negotiation IAC %d %d, want IAC DO ECHO", command, option)
	}
	server.echoAccepted = true
	return nil
}
