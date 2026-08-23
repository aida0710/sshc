package sshclient

import (
	"fmt"
	"io"
	"time"
)

// 接続の途中経過を、端末そのものへ書く。
//
// **行き先はログではなく画面である。** 繋がらなかった人・切れた人がその場で
// 読めなければ意味が無い——ログファイルは、どこにあるかを知っている人しか
// 開かない。`ssh -v` が stderr へ書くのと同じ理由で、同じ場所へ書く。
//
// **既定は無言である。** 毎回この量が流れると、シェルの最初の一画面が
// 押し流される。読みたい人だけが引き上げる。

// Verbosity は、どこまで言うかである。
type Verbosity int

const (
	// Quiet は、接続の途中を何も言わない。既定である。
	Quiet Verbosity = 0
	// Brief は `-v` に相当する。**何が起き、どこへ繋いだか**だけを言う。
	Brief Verbosity = 1
	// Detailed は `-vv` に相当する。試した鍵、通った方式、鍵の指紋、経由地。
	Detailed Verbosity = 2
	// Full は `-vvv` に相当する。取り決めた算法と、掛かった時間。
	Full Verbosity = 3
)

// MaxVerbosity は、受け付ける上限である。**設定の検証はここを見る。**
const MaxVerbosity = int(Full)

// tracer は、level 以下の話だけを書き出す。
//
// **ゼロ値が無言である。** Verbosity を渡し忘れた組み立ては、何も言わない側に
// 落ちる——うるさい側に落ちると、渡し忘れが利用者の画面に出る。
type tracer struct {
	level  Verbosity
	writer io.Writer
	// clock は掛かった時間を測る。テストが時計を止められるようにしてある。
	clock func() time.Time
}

// now は、いまを返す。
//
// **nil の tracer でも呼べる。** 途中経過を出さない道（`sshc run` や到達確認）は
// tracer を持たないまま同じ関数を通る——そこで落ちるくらいなら、時計だけは
// 誰にでも答える方がよい。
func (t *tracer) now() time.Time {
	if t == nil || t.clock == nil {
		return time.Now()
	}
	return t.clock()
}

func newTracer(level Verbosity, writer io.Writer) *tracer {
	return &tracer{level: level, writer: writer, clock: time.Now}
}

// say は、その level が求められていれば 1 行書く。
//
// **端末は CRLF を要る。** 生の \n だけを送ると、次の行が前の行の右端から
// 始まる——PTY はここを通っていないので、誰も直してくれない。
func (t *tracer) say(level Verbosity, format string, args ...any) {
	if t == nil || t.writer == nil || level > t.level {
		return
	}
	_, _ = io.WriteString(t.writer, "\r\n[sshc] "+fmt.Sprintf(format, args...)+"\r\n")
}

// announce は、level に関わらず 1 行書く。
//
// **既定が無言であることの、ただ一つの例外である。** 接続のために利用者の
// 設定に書かれたプログラムを起こすとき、それを黙って行わない。何が走るかを
// 知らないまま走ることが無いようにするためであり、繋がるまで数秒かかる理由も
// そこにある——`cloudflared` や `aws ssm` は、すぐには繋がらない。
//
// **これを二つ目にしない。** 無言を選んだ人の画面を、この一行以外で汚さない。
func (t *tracer) announce(format string, args ...any) {
	if t == nil || t.writer == nil {
		return
	}
	_, _ = io.WriteString(t.writer, "\r\n[sshc] "+fmt.Sprintf(format, args...)+"\r\n")
}

// since は、始まりからの経過を返す。Full のときだけ意味を持つ。
func (t *tracer) since(start time.Time) time.Duration { return t.now().Sub(start) }

// enabled は、この level が求められているかを答える。
//
// **組み立てが高い行のために置く。** 指紋も算法も、言わないなら計算しない。
func (t *tracer) enabled(level Verbosity) bool {
	return t != nil && t.writer != nil && level <= t.level
}
