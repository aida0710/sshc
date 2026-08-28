package sshclient

import (
	"fmt"
	"io"
	"time"

	"sshc/internal/terminal"
)

// 接続の途中経過を、端末そのものへ書く。
//
// 接続失敗または切断の診断は `ssh -v` と同様に端末の stderr へ出力する。
//
// 既定は無言である。毎回この量が流れると、シェルの最初の一画面が
// 押し流されるため、必要な場合だけ verbosity を上げる。

// Verbosity は、どこまで言うかである。
type Verbosity int

const (
	// Quiet は、接続の途中を何も言わない。既定である。
	Quiet Verbosity = 0
	// Brief は `-v` に相当する。何が起き、どこへ繋いだかだけを言う。
	Brief Verbosity = 1
	// Detailed は `-vv` に相当する。試した鍵、通った方式、鍵の指紋、経由地。
	Detailed Verbosity = 2
	// Full は `-vvv` に相当する。取り決めた算法と、掛かった時間。
	Full Verbosity = 3
)

// MaxVerbosity は、受け付ける上限である。設定の検証はここを見る。
const MaxVerbosity = int(Full)

// tracer は、設定された level 以下の診断だけを書き出す。ゼロ値は出力しない。
type tracer struct {
	level  Verbosity
	writer io.Writer
	// clock は掛かった時間を測る。テストが時計を止められるようにしてある。
	clock    func() time.Time
	progress func(terminal.ConnectionProgress)
}

// now は現在時刻を返す。nil の tracer でも呼び出せる。
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
// 端末は CRLF を要る。生の \n だけを送ると、次の行が前の行の右端から
// 始まる。PTY はここを通っていないので、誰も直してくれない。
func (t *tracer) say(level Verbosity, format string, args ...any) {
	if t == nil || t.writer == nil || level > t.level {
		return
	}
	_, _ = io.WriteString(t.writer, "\r\n[sshc] "+fmt.Sprintf(format, args...)+"\r\n")
}

// announce は verbosity に関係なく ProxyCommand の実行を 1 行表示する。
func (t *tracer) announce(format string, args ...any) {
	if t == nil || t.writer == nil {
		return
	}
	_, _ = io.WriteString(t.writer, "\r\n[sshc] "+fmt.Sprintf(format, args...)+"\r\n")
}

// since は、始まりからの経過を返す。Full のときだけ意味を持つ。
func (t *tracer) since(start time.Time) time.Duration { return t.now().Sub(start) }

// enabled は、この level の診断が有効かを返す。
func (t *tracer) enabled(level Verbosity) bool {
	return t != nil && t.writer != nil && level <= t.level
}

func (t *tracer) stage(phase string, target Target, hop, hops int) {
	if t == nil || t.progress == nil {
		return
	}
	t.progress(terminal.ConnectionProgress{
		Phase: phase, Alias: target.Alias, HostName: target.HostName,
		User: target.User, Hop: hop, Hops: hops,
	})
}
