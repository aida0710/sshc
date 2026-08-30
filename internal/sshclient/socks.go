package sshclient

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

// SOCKS5 の符号。RFC 1928 のもの。
const (
	socksVersion5 = 0x05
	socksNoAuth   = 0x00
	socksConnect  = 0x01

	socksIPv4   = 0x01
	socksDomain = 0x03
	socksIPv6   = 0x04

	socksSucceeded         = 0x00
	socksCommandNotAllowed = 0x07
)

// ErrUnsupportedSOCKS は、このクライアントが話さない SOCKS の要求を報告する。
var ErrUnsupportedSOCKS = errors.New("this client speaks only SOCKS5 CONNECT")

// maxSOCKSName は、要求に載せられるホスト名の長さである。プロトコルの上限。
const maxSOCKSName = 255

// socksNegotiationTimeout は、SOCKS5 の greeting と CONNECT 要求を受け取る期限。
// 通常は数バイトのローカル通信だが、起動直後なども考慮して余裕を持たせる。
const socksNegotiationTimeout = 10 * time.Second

// readSOCKS5 は、SOCKS5 のやり取りを受けて宛先を返す。
//
// CONNECT だけを受ける。BIND も UDP ASSOCIATE も、このクライアントが
// 持たない機能である。持たないものを受け付けて暗黙に失敗するより、その場で断る。
//
// 認証は「認証なし」だけを受ける。ループバックは接続元を同じ機械に限定するが、
// 同じ OS ユーザーに限定するものではない。そのため共有機械では、別ユーザーからも
// このプロキシへ到達できることを前提に扱う。
func readSOCKS5(conn net.Conn) (string, error) {
	if err := conn.SetReadDeadline(time.Now().Add(socksNegotiationTimeout)); err != nil {
		return "", err
	}
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != socksVersion5 {
		return "", ErrUnsupportedSOCKS
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return "", err
	}
	offered := false
	for _, method := range methods {
		if method == socksNoAuth {
			offered = true
		}
	}
	if !offered {
		_, _ = conn.Write([]byte{socksVersion5, 0xff})
		return "", ErrUnsupportedSOCKS
	}
	if _, err := conn.Write([]byte{socksVersion5, socksNoAuth}); err != nil {
		return "", err
	}

	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return "", err
	}
	if request[0] != socksVersion5 {
		return "", ErrUnsupportedSOCKS
	}
	if request[1] != socksConnect {
		_, _ = conn.Write(socksReply(socksCommandNotAllowed))
		return "", ErrUnsupportedSOCKS
	}

	host, err := readSOCKSAddress(conn, request[3])
	if err != nil {
		return "", err
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(portBytes)

	// 成功を先に返す。SOCKS5 は、こちらが繋いだあとに返すことも許すが、
	// 返してから繋ぐ実装が広く使われており、繋がらなかった場合は接続を閉じる
	// ことで伝わる。
	if _, err := conn.Write(socksReply(socksSucceeded)); err != nil {
		return "", err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func readSOCKSAddress(conn net.Conn, kind byte) (string, error) {
	switch kind {
	case socksIPv4:
		address := make([]byte, 4)
		if _, err := io.ReadFull(conn, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	case socksIPv6:
		address := make([]byte, 16)
		if _, err := io.ReadFull(conn, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	case socksDomain:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return "", err
		}
		if length[0] == 0 || int(length[0]) > maxSOCKSName {
			return "", ErrUnsupportedSOCKS
		}
		name := make([]byte, int(length[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			return "", err
		}
		return string(name), nil
	default:
		return "", ErrUnsupportedSOCKS
	}
}

// socksReply は、宛先を持たない返事を組み立てる。
//
// 0.0.0.0:0 を載せるのは、こちらが使った送り元をクライアントへ明かさないため
// である。RFC はここに実際の bind 先を書くことを認めているが、CONNECT の
// クライアントはこの欄を読まない。
func socksReply(status byte) []byte {
	return []byte{socksVersion5, status, 0x00, socksIPv4, 0, 0, 0, 0, 0, 0}
}
