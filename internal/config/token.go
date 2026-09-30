package config

import (
	"errors"
	"strings"
)

// Argument は、ディレクティブの引数ひとつと、それを生んだ正確なバイト列の組。
// Lead は引数の前にある空白を保持するので、解析した行を 1 バイトも違わず
// レンダリングし直せる。
//
// 行末のコメントも Argument ひとつとして持つ。Raw と Value はどちらも '#' から
// 行末の空白の手前までである。Values はそこで止まる。
type Argument struct {
	Lead  string
	Raw   string
	Value string
}

// ErrUnquotableValue は、ssh_config で表現できない値を報告する。設定ファイルは
// 行単位で読まれるので、改行と NUL を含む値には表記が無い。壊すのではなく断る。
var ErrUnquotableValue = errors.New("value cannot be quoted for an OpenSSH configuration")

// RenderArgument は、OpenSSH の argv_split が value そのものに読み戻す表記で、
// 値をひとつ書き出す。
//
// 引用が要らない値はそのまま書く。要る値は二重引用符で囲み、中の二重引用符と、
// エスケープとして読まれてしまうバックスラッシュだけを前に '\' を付けて書く。
func RenderArgument(lead, value string) (Argument, error) {
	if strings.ContainsAny(value, "\n\r\x00") {
		return Argument{}, ErrUnquotableValue
	}
	raw := value
	if needsQuoting(value) {
		raw = quoteArgument(value)
	}
	return Argument{Lead: lead, Raw: raw, Value: value}, nil
}

// needsQuoting は、そのまま書くと argv_split が別の値に読む値かを報告する。
//
// 先頭の '=' は、最初の引数ならキーワードとの区切りとして読まれる。末尾の '\' は、
// 次の引数との間の空白をエスケープする。どちらも、どの位置に書かれるかをここでは
// 知らないので、常に引用する。
func needsQuoting(value string) bool {
	return value == "" ||
		strings.ContainsAny(value, " \t\"'") ||
		strings.HasPrefix(value, "#") ||
		strings.HasPrefix(value, "=") ||
		strings.HasSuffix(value, `\`) ||
		strings.Contains(value, `\\`)
}

// quoteArgument は value を二重引用符で囲む。
//
// 引用の中で argv_split がエスケープと読むのは \" \' \\ の三つだけである。その形に
// なるバックスラッシュと、閉じ引用符の直前に来る末尾のバックスラッシュを二重にする。
// それ以外のバックスラッシュは書いたまま読まれるので、Windows のパスはそのまま残る。
func quoteArgument(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch {
		case character == '"':
			builder.WriteString(`\"`)
		case character == '\\' && (index+1 == len(value) || strings.IndexByte(`\"'`, value[index+1]) >= 0):
			builder.WriteString(`\\`)
		default:
			builder.WriteByte(character)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

// splitArguments は、ディレクティブ行の引数部分を OpenSSH 8.7 以降の argv_split
// （misc.c）と同じ規則で分割する。
//
// 語は引用の外の空白で区切る。二重引用符と一重引用符は語のどこでも引用を開閉し、
// 引用符そのものは値に入らない。バックスラッシュは \" \' \\ と、引用の外の
// "\ "（空白）だけをエスケープとして読み、それ以外は文字どおり残す。語の先頭の
// '#' から後ろはコメントである。
//
// 閉じない引用は非構造化として報告する（ok == false）。OpenSSH はその行を
// 「invalid quotes」として設定全体ごと断る。呼び出し側は意味を推測する代わりに、
// その行を逐語的に保持する。
func splitArguments(input string) (arguments []Argument, trailing string, ok bool) {
	index := 0
	for {
		leadStart := index
		for index < len(input) && isSpace(input[index]) {
			index++
		}
		lead := input[leadStart:index]
		if index == len(input) {
			return arguments, lead, true
		}
		if input[index] == '#' {
			end := len(strings.TrimRight(input, " \t"))
			comment := input[index:end]
			arguments = append(arguments, Argument{Lead: lead, Raw: comment, Value: comment})
			return arguments, input[end:], true
		}
		value, end, closed := readWord(input, index)
		if !closed {
			return nil, "", false
		}
		arguments = append(arguments, Argument{Lead: lead, Raw: input[index:end], Value: value})
		index = end
	}
}

// readWord は、start から始まる語ひとつを argv_split の規則で読み、引用とエスケープを
// 外した値と、語の直後の位置を返す。引用が閉じないまま入力が尽きたら closed は false。
func readWord(input string, start int) (value string, end int, closed bool) {
	var builder strings.Builder
	var quote byte
	index := start
	for index < len(input) {
		character := input[index]
		switch {
		case character == '\\' && index+1 < len(input) && isEscaped(input[index+1], quote):
			index++
			builder.WriteByte(input[index])
		case quote == 0 && isSpace(character):
			return builder.String(), index, true
		case quote == 0 && (character == '"' || character == '\''):
			quote = character
		case quote != 0 && character == quote:
			quote = 0
		default:
			builder.WriteByte(character)
		}
		index++
	}
	return builder.String(), index, quote == 0
}

// isEscaped は、バックスラッシュの次の文字をエスケープとして読むかを報告する。
// 空白は引用の外でだけエスケープになる。引用の中の空白はもともと語を区切らない。
func isEscaped(next, quote byte) bool {
	switch next {
	case '\'', '"', '\\':
		return true
	case ' ':
		return quote == 0
	}
	return false
}

func isSpace(character byte) bool {
	return character == ' ' || character == '\t'
}
