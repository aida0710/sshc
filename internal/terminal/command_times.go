package terminal

import (
	"strconv"
	"time"
)

// ブラウザは、それぞれのコマンドがいつ始まり、どれだけかかったかを出す。ずっとあとでつないだときに流し直す
// 出力についても出す（web/src/terminal/commandBlocks.ts）。シェルの OSC 133;C と OSC 133;D は時刻を持たない
// ので、エンジンは、それぞれを見た時刻を、それを含んでいたかたまりのあとに、出力へ書く。
//
//	OSC 7742 ; C ; <Unix のミリ秒> BEL   OSC 133;C（コマンドが始まった）
//	OSC 7742 ; D ; <Unix のミリ秒> BEL   OSC 133;D（コマンドが終わった）
//
// 番号は sshc 独自のものである。ほかのターミナルは知らない OSC を無視し、PlainTextFrom は、CLI が読む文字から
// これを除く。
const commandTimeOSC = "7742"

// commandTime は、エンジンが、コマンドの開始（'C'）か終了（'D'）を見た時刻である。
type commandTime struct {
	kind byte
	at   time.Time
}

// commandTimesSequence は、シェルが報告した順に、時刻を書く。
func commandTimesSequence(times []commandTime) []byte {
	var sequence []byte
	for _, mark := range times {
		sequence = append(sequence, "\x1b]"+commandTimeOSC+";"...)
		sequence = append(sequence, mark.kind, ';')
		sequence = strconv.AppendInt(sequence, mark.at.UnixMilli(), 10)
		sequence = append(sequence, '\a')
	}
	return sequence
}
