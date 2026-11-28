package sessionlog

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"sshc/internal/redact"
)

// redaction は、シェルが映したシークレットの代わりにログへ書く印である。スニペットの伏せた文と同じ印にする。
const redaction = "[secret]"

// minimumSecretLineLength は、改行を含む値を行に分けたとき、1行だけでも伏せる長さ（字数）の下限である。
// 引用しない改行はコマンドを区切るので、シェルは値の行を別々の行に映し、値そのものとは一致しない。
// パスワードの最短の長さとしてよく使われる 8 字より短い行は、ターミナルのふつうの語と見分けがつかず、
// 伏せるとログが読めなくなる。
const minimumSecretLineLength = 8

// lineRedactor は、画面を出た行のシークレットを伏せ、ログに書く順に返す。
//
// ログ用の画面は行を1行ずつ返し、右端で折り返した続きも別の行になる。値が折り返しをまたぐと、どちらの行にも
// 値そのものは無い。そこで、手元の行をつないで値を探し、またいだ部分を両方の行で伏せる。行は、そのあとに
// 最も長い値の分の文字が続くまで手元に留める。vt の行は折り返したかどうかを持たないので、改行で終わった行も
// つないで探す。別々の行にたまたま値の前と後ろが並ぶと伏せることになるが、伏せ過ぎは漏れより害が小さい。
type lineRedactor struct {
	secrets []redact.Secret
	// longest は、secrets のうち最も長い値のバイト数である。
	longest int
	held    []heldLine
	// joined は、手元の行をつないだ文字列を作るたびに使い回す場所である。
	joined []byte
}

// heldLine は、まだログに書いていない、画面を出た行である。
type heldLine struct {
	text string
	// blanks は、text の右から画面の右端までの空いた桁の数である。画面は行末の空白を落として返すので、
	// 値の中の空白が右端に来ると、行をそのままつないでも値と一致しない。
	blanks int
	// hidden は、text のうち伏せるバイトである。nil なら伏せるところは無い。
	hidden []bool
	// notes は、この行のあとに書く [sshc] の行である。
	notes []string
}

// addSecrets は、これから書く行で伏せる値を足す。手元に留まっている行も、まだ書いていないので伏せる。
// 同じ値は、再接続のたびの起動スニペットや、同じコマンドを送るたびに渡される。重ねて持つと、行を受け取る
// たびに同じ値を何度も探すので、持っている値は足さない。
func (r *lineRedactor) addSecrets(values []string) {
	for _, value := range values {
		for _, shown := range secretForms(value) {
			if r.hasSecret(shown) {
				continue
			}
			r.secrets = append(r.secrets, redact.Secret{Value: []byte(shown)})
			r.longest = max(r.longest, len(shown))
		}
	}
	r.hideInHeldLines(0)
}

// hasSecret は、value をすでに伏せる値として持っているかを返す。
func (r *lineRedactor) hasSecret(value string) bool {
	return slices.ContainsFunc(r.secrets, func(secret redact.Secret) bool { return string(secret.Value) == value })
}

// secretForms は、value がログに映りうる形を返す。改行を含まない値は、そのまま返す。画面の行は改行を
// 含まないので、改行を含む値は行に分けて（前後の空白を除いて）返す。文字のある行が1つだけならその行を、
// 2つ以上なら minimumSecretLineLength 以上の行を返す。考え方は VPN の docker logs で設定ファイルの鍵の
// 行を伏せる openVPNInlineLines と同じ。
func secretForms(value string) []string {
	if value == "" {
		return nil
	}
	if !strings.ContainsAny(value, "\r\n") {
		return []string{value}
	}
	var lines []string
	for _, line := range strings.FieldsFunc(value, isLineBreak) {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 1 {
		// 改行で終わるだけの値は、その1行が値そのものなので、短くても伏せる。
		return lines
	}
	var forms []string
	for _, line := range lines {
		if utf8.RuneCountInString(line) >= minimumSecretLineLength {
			forms = append(forms, line)
		}
	}
	return forms
}

func isLineBreak(character rune) bool { return character == '\n' || character == '\r' }

// add は、width 桁の画面を出た行 text を受け取り、書いてよくなった行を伏せて返す。
func (r *lineRedactor) add(text string, width int) []string {
	if len(r.secrets) == 0 {
		return []string{text}
	}
	r.held = append(r.held, heldLine{text: text, blanks: max(width-ansi.StringWidth(text), 0)})
	r.hideInHeldLines(len(r.held) - 1)
	if text == "" {
		// 折り返した行は右端まで埋まり、続きの行には文字がある。空の行をまたいで続く値は無い。
		return r.releaseAll()
	}
	return r.releaseReady()
}

// addNote は、[sshc] の行 note を、これまでに受け取った行のあとに書くよう預かり、いま書いてよければ返す。
func (r *lineRedactor) addNote(note string) []string {
	if len(r.held) == 0 {
		return []string{note}
	}
	last := &r.held[len(r.held)-1]
	last.notes = append(last.notes, note)
	return nil
}

// releaseReady は、あとに続く文字が、最も長い値より1バイト少ない長さに届いた行を伏せて返す。そこまで続けば、
// その行で始まる値は、どれも手元の行の中で終わっている。
func (r *lineRedactor) releaseReady() []string {
	following := 0
	for _, line := range r.held {
		following += len(line.text)
	}
	ready := 0
	for ready < len(r.held) {
		following -= len(r.held[ready].text)
		if following < r.longest-1 {
			break
		}
		ready++
	}
	return r.release(ready)
}

// releaseAll は、手元の行をすべて伏せて返す。ログを閉じるときと、空の行が来たときに使う。
func (r *lineRedactor) releaseAll() []string {
	return r.release(len(r.held))
}

// release は、手元の先頭の count 行を、伏せた文字と、そのあとに書く [sshc] の行に直して返す。
func (r *lineRedactor) release(count int) []string {
	var lines []string
	for _, line := range r.held[:count] {
		lines = append(lines, line.redacted())
		lines = append(lines, line.notes...)
	}
	r.held = append(r.held[:0], r.held[count:]...)
	return lines
}

// hideInHeldLines は、手元の行をつないだ文字列で、from 番目から後ろの行にかかる値を探し、見つけた範囲を行ごとに
// 印す。それより前の行の中で終わる値は、その行を受け取ったときに探してある。行の間に右端までの空白を入れて
// つなぐ形と、入れずにつなぐ形の両方で探す。前者は右端に来た値の空白を戻す。後者は、幅が広がった画面の行や、
// 右端に入らなかった全角の文字で折り返した行に当たる。
func (r *lineRedactor) hideInHeldLines(from int) {
	r.hideInJoinedLines(from, false)
	r.hideInJoinedLines(from, true)
}

func (r *lineRedactor) hideInJoinedLines(from int, withBlanks bool) {
	if from >= len(r.held) {
		return
	}
	// from 番目の行にかかる値は、その行の前の longest-1 バイトより前からは始まらない。
	first := from
	for reach := 0; first > 0 && reach < r.longest-1; {
		first--
		reach += r.held[first].joinedLength(withBlanks)
	}
	joined := r.joined[:0]
	starts := make([]int, 0, len(r.held)-first)
	for _, line := range r.held[first:] {
		starts = append(starts, len(joined))
		joined = append(joined, line.text...)
		if withBlanks {
			for range line.blanks {
				joined = append(joined, ' ')
			}
		}
	}
	r.joined = joined
	for _, span := range redact.Spans(joined, r.secrets) {
		for index, start := range starts {
			r.held[first+index].hide(span.Start-start, span.End-start)
		}
	}
}

// joinedLength は、手元の行をつないだ文字列の中で、この行が占めるバイト数である（hideInJoinedLines）。
func (line heldLine) joinedLength(withBlanks bool) int {
	if withBlanks {
		return len(line.text) + line.blanks
	}
	return len(line.text)
}

// hide は、行の頭から数えた [start, end) のうち、text に入る部分を伏せる。
func (line *heldLine) hide(start, end int) {
	start, end = max(start, 0), min(end, len(line.text))
	if start >= end {
		return
	}
	if line.hidden == nil {
		line.hidden = make([]bool, len(line.text))
	}
	for index := start; index < end; index++ {
		line.hidden[index] = true
	}
}

// redacted は、text の伏せる部分を、続いた部分ごとに1つの印に置き換えたものである。
func (line heldLine) redacted() string {
	if line.hidden == nil {
		return line.text
	}
	var built strings.Builder
	for index := 0; index < len(line.text); {
		if !line.hidden[index] {
			built.WriteByte(line.text[index])
			index++
			continue
		}
		built.WriteString(redaction)
		for index < len(line.text) && line.hidden[index] {
			index++
		}
	}
	return built.String()
}
