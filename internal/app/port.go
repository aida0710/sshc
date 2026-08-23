package app

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
)

// 受け口の番号を決める。
//
// **番号は秘密ではない。** 走査すれば見つかるし、handoff にも書いてある。守って
// いるのは実行ごとの bootstrap 秘密・CSRF トークン・セッションであって、番号では
// ない。だから固定できることは、安全を下げる取引ではない。
//
// **それでも既定は無作為である。** 固定した番号は、sshc より先に別のプロセスが
// 握れる——そこへ偽の画面を出されると、マスターパスワードを差し出す相手が
// すり替わる。当てにくい番号は、その競争を面倒にするだけの価値がある。
//
// 選ぶのは 30000 以上にする。**1024 未満は特権が要り、その上も名前の付いた
// サービスで混み合っている。** OS に任せる（`:0`）と、機械ごとに違う一時ポートの
// 範囲へ落ちる——Linux は 32768 から、macOS は 49152 から。自分で決めれば、
// どの機械でも同じ範囲から出る。

const (
	// LowestPort は、無作為に選ぶ範囲の下端である。
	LowestPort = 30000
	// HighestPort は、上端である。
	HighestPort = 60000
	// portAttempts は、埋まっていた番号を引き直す回数である。
	//
	// **無限に引き直さない。** 3 万通りの中で 32 回続けて外れるなら、外れて
	// いるのは運ではなく前提である（名前空間ごと塞がれている、など）。
	portAttempts = 32
)

// ErrNoFreePort は、どの番号も掴めなかったことを報告する。
var ErrNoFreePort = errors.New("no free port was found for the engine")

// listenLoopback は、決めた番号で受け口を開く。
//
// wanted が 0 なら無作為に選ぶ。0 でなければ**その番号だけを試す** ——選んだ人が
// 居るのに黙って別の番号へ逃げると、その人がブラウザに打ち込む綴りが外れる。
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
//
// **crypto/rand から引く。** 番号は秘密ではないが、当てにくいことには意味が
// ある——先に握られる競争を面倒にするのがその値である。予測できる数列から
// 選べば、その面倒がそのまま消える。
func randomBelow(limit int) (int, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	return int(binary.BigEndian.Uint64(raw[:]) % uint64(limit)), nil
}
