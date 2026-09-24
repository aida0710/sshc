// Package connectionlog は、接続の途中経過を、接続ログ（Terminal と CLI の
// [sshc][debug1]〜[debug3]）へ届ける口である。
//
// 接続ログを書くのは sshclient である。VPN の経路のように、sshclient の外で
// 輸送を用意する部品は、ctx に載った Writer を通してここへ書く。Writer が
// 載っていなければ何も書かない。
package connectionlog

import (
	"context"
	"fmt"
)

// Level は、その行を出す深さである。sshclient.Verbosity と同じ値を使う。
type Level int

const (
	// Brief は `-v` に相当する。何が起き、どこへ繋いだか。
	Brief Level = 1
	// Detailed は `-vv` に相当する。使った設定、掛かった時間、失敗の詳細。
	Detailed Level = 2
	// Full は `-vvv` に相当する。実行したコマンド、途中の出力のすべて。
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
