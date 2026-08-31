package app

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
)

// listen port を決定する。

const (
	// DefaultPort gives bookmarks and installed web apps a stable origin.
	DefaultPort = 54447
	// LowestPort は、無作為に選ぶ範囲の下端である。
	LowestPort = 30000
	// HighestPort は、上端である。
	HighestPort = 60000
	// portAttempts は、埋まっていた番号を引き直す回数である。
	portAttempts = 32
)

// ErrNoFreePort は、どの番号も掴めなかったことを報告する。
var ErrNoFreePort = errors.New("no free port was found for the engine")

// listenLoopback は指定ポートで loopback listener を開く。
func listenLoopback(listen ListenFunc, wanted int, random func(int) (int, error)) (net.Listener, error) {
	if wanted != 0 {
		listener, err := listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(wanted)))
		if err != nil {
			return nil, fmt.Errorf("listen on port %d: %w", wanted, err)
		}
		return listener, nil
	}
	for attempt := 0; attempt < portAttempts; attempt++ {
		port, err := random(HighestPort - LowestPort + 1)
		if err != nil {
			return nil, err
		}
		listener, err := listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(LowestPort+port)))
		if err == nil {
			return listener, nil
		}
	}
	return nil, ErrNoFreePort
}

// randomBelow は、0 以上 limit 未満の数をひとつ返す。
func randomBelow(limit int) (int, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint64(raw[:]) % uint64(limit)), nil
}
