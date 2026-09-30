// Package connectionlog は、接続の途中経過を、接続ログ（Terminal と CLI の
// [sshc] と [sshc][debug1]〜[debug3]）へ届ける口である。
//
// 接続ログを書くのは sshclient である。VPN の経路のように、sshclient の外で
// 輸送を用意する部品は、ctx に載った Writer を通してここへ書く。Writer が
// 載っていなければ何も書かない。
package connectionlog

import (
	"context"
	"fmt"
	"time"
)

// Level は、その行を出す深さである。接続ログの設定（どの深さまで書くか）も
// この型で表し、設定の深さ以下の行を書く。設定が Notice なら `[sshc]` の行だけを書く。
type Level int

const (
	// Notice は、接続ログの設定に関係なく出す行である（`[sshc]` で始まる）。
	// 時間のかかる段階と、利用者が何かをする必要がある段階を知らせる。
	Notice Level = 0
	// Brief は `-v` に相当する。何が起き、どこへ繋いだか。
	Brief Level = 1
	// Detailed は `-vv` に相当する。使った設定、掛かった時間、失敗の詳細。SSH では、
	// 試した鍵とそのフィンガープリント、通った方式、ホスト鍵の照合結果、経由ホスト、
	// PTY と環境変数の要求。
	Detailed Level = 2
	// Full は `-vvv` に相当する。実行したコマンド、途中の出力のすべて。SSH では、
	// 提示したアルゴリズム、ssh-agent の鍵の一覧、keyboard-interactive のプロンプトごとの扱い、
	// 通った経路のアドレス。
	Full Level = 3
)

// Writer は、接続ログの書き先である。
type Writer interface {
	// Enabled は、その深さの行を書くかを返す。重い処理（ログの取り寄せなど）を
	// 書かない深さで行わないために使う。
	Enabled(level Level) bool
	Write(level Level, message string)
}

type writerKey struct{}

// With は、writer を書き先に足した ctx を返す。すでに書き先があれば、両方へ書く。
func With(ctx context.Context, writer Writer) context.Context {
	if previous, present := ctx.Value(writerKey{}).(Writer); present {
		writer = both{first: writer, second: previous}
	}
	return context.WithValue(ctx, writerKey{}, writer)
}

// Muted は、接続ログへ何も書かない ctx を返す。短い間隔で何度も繰り返す処理
// （コンテナの状態の確認など）で使う。1回ずつ書くと、接続ログと記録が埋まる。
func Muted(ctx context.Context) context.Context {
	return context.WithValue(ctx, writerKey{}, nil)
}

// Enabled は、ctx の書き先がその深さの行を書くかを返す。書き先が無ければ false。
func Enabled(ctx context.Context, level Level) bool {
	writer, present := ctx.Value(writerKey{}).(Writer)
	return present && writer.Enabled(level)
}

// Say は、ctx の書き先へ 1 行書く。書き先が無いか、その深さを書かないなら何もしない。
func Say(ctx context.Context, level Level, format string, args ...any) {
	writer, present := ctx.Value(writerKey{}).(Writer)
	if !present || !writer.Enabled(level) {
		return
	}
	writer.Write(level, fmt.Sprintf(format, args...))
}

// ProgressSkipper は、途中の出力（Progress）を書かない書き先が満たす。経路ごとの
// 記録は行数に上限があり、docker build のように数百行になる途中の出力で、ほかの
// 行を押し出さないために満たす。
type ProgressSkipper interface {
	SkipsProgress()
}

// Progress は、長い処理の途中の出力（docker build の行など）を 1 行書く。書くのは、
// その場で接続を見ている書き先（Terminal と CLI）だけで、ProgressSkipper を満たす
// 書き先には書かない。
func Progress(ctx context.Context, level Level, format string, args ...any) {
	writer, present := ctx.Value(writerKey{}).(Writer)
	if !present {
		return
	}
	writeProgress(writer, level, fmt.Sprintf(format, args...))
}

func writeProgress(writer Writer, level Level, message string) {
	if pair, isPair := writer.(both); isPair {
		writeProgress(pair.first, level, message)
		writeProgress(pair.second, level, message)
		return
	}
	if _, skips := writer.(ProgressSkipper); skips || !writer.Enabled(level) {
		return
	}
	writer.Write(level, message)
}

// Elapsed は、掛かった時間を接続ログに出す細かさへ丸める。ms で丸めると 1ms 未満が
// 「0s」になり、測っていないように見えるので、そこだけ µs で丸める。
func Elapsed(duration time.Duration) time.Duration {
	if duration < time.Millisecond {
		return duration.Round(time.Microsecond)
	}
	return duration.Round(time.Millisecond)
}

// both は、2 つの書き先へ同じ行を書く。
type both struct {
	first, second Writer
}

func (writer both) Enabled(level Level) bool {
	return writer.first.Enabled(level) || writer.second.Enabled(level)
}

func (writer both) Write(level Level, message string) {
	if writer.first.Enabled(level) {
		writer.first.Write(level, message)
	}
	if writer.second.Enabled(level) {
		writer.second.Write(level, message)
	}
}
