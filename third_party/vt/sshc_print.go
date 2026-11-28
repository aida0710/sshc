package vt

// sshc がこのパッケージの写しに足したもの（README.sshc.md）。出力の字を画面のマスに書く。上流の handleGrapheme は、
// 右端の1桁に入らない幅2の字を空白にして落とし、右端の2桁をちょうど埋めた幅2の字のあとで折り返しを待たない
// （次の字がその字を消す）。ここでは、ブラウザのターミナル（xterm.js の InputHandler の print）と同じに書く。

import (
	"io"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// parserHandler は、パーサーが読んだものを渡す先である。字は、続く字が結合文字でないと分かるまで e.grapheme に
// 持っておく。上流の流れは、制御文字やシーケンスを扱う前に、持っておいた字を上流の handleGrapheme で書く。
// そこで、どれを扱う前にも、先にこちらの書き方（flushClusters）で書き出しておく。
func (e *SnapshotEmulator) parserHandler() ansi.Handler {
	return ansi.Handler{
		Print: e.print,
		Execute: func(code byte) {
			e.flushClusters()
			e.handleControl(code)
		},
		HandleCsi: func(command ansi.Cmd, params ansi.Params) {
			e.flushClusters()
			e.handleCsi(command, params)
		},
		HandleEsc: func(command ansi.Cmd) {
			e.flushClusters()
			e.handleEsc(command)
		},
		HandleDcs: func(command ansi.Cmd, params ansi.Params, data []byte) {
			e.flushClusters()
			e.handleDcs(command, params, data)
		},
		HandleOsc: func(command int, data []byte) {
			e.flushClusters()
			e.handleOsc(command, data)
		},
		HandleApc: func(data []byte) {
			e.flushClusters()
			e.handleApc(data)
		},
		HandlePm: func(data []byte) {
			e.flushClusters()
			e.handlePm(data)
		},
		HandleSos: func(data []byte) {
			e.flushClusters()
			e.handleSos(data)
		},
	}
}

// readOutput は、上流の Emulator の Write と同じに output を読む。読み終えたときに持っておいた字を書き出すのも、
// こちらの書き方（flushClusters）にする。
func (e *SnapshotEmulator) readOutput(output []byte) (int, error) {
	if e.closed {
		return 0, io.ErrClosedPipe
	}
	for _, b := range output {
		e.parser.Advance(b)
	}
	e.flushClusters()
	return len(output), nil
}

// print は、上流の handlePrint と同じに字を持っておく。印字できる ASCII の字は、前の字のまとまり（結合文字を付けた
// 字など）に続かないので、来たら持っておいた字を書き出す。ASCII の字のあとに ASCII の字が来るのがいちばん多く、
// そのときは字の区切りを調べずに書く（putHeldASCII）。
func (e *SnapshotEmulator) print(r rune) {
	if !printableASCII(r) {
		e.grapheme = utf8.AppendRune(e.grapheme, r)
		return
	}
	if len(e.grapheme) == 1 && printableASCII(rune(e.grapheme[0])) {
		e.putHeldASCII(e.grapheme[0])
		e.grapheme[0] = byte(r)
		return
	}
	e.flushClusters()
	e.grapheme = append(e.grapheme, byte(r))
}

// asciiContents は、印字できる ASCII の字のマスの中身を、一度だけ作ったものである。上流の print は、字のたびに
// 新しい文字列を作る。
var asciiContents = func() (contents [ansi.DEL]string) {
	for c := byte(ansi.SP); c < ansi.DEL; c++ {
		contents[c] = string(rune(c))
	}
	return contents
}()

// putHeldASCII は、持っておいた印字できる ASCII の字 c を書く。文字の集合で写さない字は、字のたびに新しい文字列を
// 作らずに書く。
func (e *SnapshotEmulator) putHeldASCII(c byte) {
	if e.charsets[e.gl] != nil || e.gsingle != 0 {
		e.putGrapheme(asciiContents[c], 1)
		return
	}
	e.lastChar = rune(c)
	e.putNarrowCell(asciiContents[c])
}

// flushClusters は、上流の flushGrapheme と同じに、持っておいた字を字のまとまりごとに書く。
func (e *SnapshotEmulator) flushClusters() {
	clusters := e.grapheme
	for len(clusters) > 0 {
		cluster, width := ansi.FirstGraphemeCluster(clusters, ansi.GraphemeWidth)
		if len(cluster) == 0 {
			break
		}
		e.putGrapheme(string(cluster), width)
		clusters = clusters[len(cluster):]
	}
	e.grapheme = e.grapheme[:0]
}

// putGrapheme は、幅 width の字のまとまり content を、カーソルの位置に書き、カーソルを進める。文字の集合での
// 写し方は上流の handleGrapheme と同じである。右端に入らない字（moveCursorToFit）と、右端の2桁をちょうど埋めた字の
// あとの折り返しの待ち（atPhantom）は、xterm.js と同じにする。
func (e *SnapshotEmulator) putGrapheme(content string, width int) {
	screen := e.scr
	if e.atPhantom && e.autowrap {
		e.wrapToNextRow()
	}
	// 1字だけの文字の集合の切り替え（SS2・SS3）は、どの字が来ても次の1字にだけ効く。
	single := e.gsingle
	e.gsingle = 0
	cell := uv.Cell{Content: content, Width: width, Style: screen.cur.Pen, Link: screen.cur.Link}
	if mapped, ok := e.mappedCharacter(content, single); ok {
		cell.Content, cell.Width = mapped, 1
	}
	if cell.Width == 1 && len(content) == 1 {
		e.lastChar, _ = utf8.DecodeRuneInString(content)
	}
	if !e.moveCursorToFit(cell.Width) {
		return
	}
	x, y := screen.cur.X, screen.cur.Y
	screen.SetCell(x, y, &cell)
	// 幅0の字も、カーソルのマスを使ったものとして、右端なら折り返しを待つ（上流と同じ）。
	lastColumn := x + max(cell.Width, 1) - 1
	e.atPhantom = e.autowrap && lastColumn >= screen.Width()-1
	if e.atPhantom {
		x = screen.Width() - 1
	} else {
		x += cell.Width
	}
	screen.setCursor(x, y, false)
}

// moveCursorToFit は、カーソルの位置から幅 width の字が右端に入らないとき、xterm.js と同じにする。折り返しが
// 有効なら、右端までのマスを空け（wrapGapCell）、カーソルを次の行の頭に移す。無効なら、カーソルを右端に置く。
// そのとき字は書かず、右端のマスもそのままにする。xterm.js は、続けて書く字の並びの先頭でカーソルが幅2の字の
// 右半分にあると、その字を消す。並びはエスケープシーケンスのほか、ブラウザに渡った出力の区切りでも切れて
// ここからは分からないので、消さない側にそろえる（README.sshc.md）。字を書いてよいかを返す。行の頭からでも
// 入らない字（1桁の画面の幅2の字）は、上流と同じにそのまま書く。
func (e *SnapshotEmulator) moveCursorToFit(width int) bool {
	screen := e.scr
	x, y := screen.cur.X, screen.cur.Y
	if x == 0 || x+width <= screen.Width() {
		return true
	}
	if !e.autowrap {
		// 折り返しを切る前に右端で待っていても、カーソルは右端のマスに戻り、もう待たない。
		screen.setCursor(screen.Width()-1, y, false)
		e.atPhantom = false
		return false
	}
	gap := wrapGapCell(screen.cur.Pen, screen.cur.Link)
	for column := x; column < screen.Width(); column++ {
		screen.SetCell(column, y, &gap)
	}
	e.wrapToNextRow()
	return true
}

// putNarrowCell は、幅1の字 content を、handleGrapheme と同じに書く。行の端で折り返し、カーソルを進める。
// 文字の集合で写すのは呼ぶ側である。
func (e *SnapshotEmulator) putNarrowCell(content string) {
	screen := e.scr
	if e.atPhantom && e.autowrap {
		e.wrapToNextRow()
	}
	x, y := screen.cur.X, screen.cur.Y
	cell := uv.Cell{Content: content, Width: 1, Style: screen.cur.Pen, Link: screen.cur.Link}
	// 幅1のマスの上には、先に消す幅2の字が無い。バッファの SetCell が足すのはその消し方と、行を描き直す印だけで、
	// 印はここでは使わない。
	if row := screen.buf.Lines[y]; x < len(row) && row[x].Width == 1 {
		row[x] = cell
	} else {
		screen.SetCell(x, y, &cell)
	}
	e.atPhantom = e.autowrap && x >= screen.Width()-1
	if !e.atPhantom {
		x++
	}
	screen.setCursor(x, y, false)
}

// wrapToNextRow は、カーソルを次の行の頭に移す。下の余白の行なら画面を流す。上流の handleGrapheme の折り返しと
// 同じ画面になる。
func (e *SnapshotEmulator) wrapToNextRow() {
	if !e.scrollUpAtBottom() {
		e.index()
	}
	e.scr.setCursor(0, e.scr.cur.Y, false)
}

// 1字だけの文字の集合の切り替えが選ぶ集合の番号である。SS2 は G2 を、SS3 は G3 を選ぶ（上流の handlers.go）。
const (
	singleShiftG2 = 2
	singleShiftG3 = 3
)

// mappedCharacter は、1バイトの字 content を、使っている文字の集合で写した字を返す。single は、1字だけの文字の
// 集合の切り替え（SS2・SS3）で選んだ集合の番号である。写さないときは false を返す。
func (e *SnapshotEmulator) mappedCharacter(content string, single int) (string, bool) {
	if len(content) != 1 {
		return "", false
	}
	c := content[0]
	var charset CharSet
	switch {
	case single == singleShiftG2 || single == singleShiftG3:
		charset = e.charsets[single]
	case c < 128:
		charset = e.charsets[e.gl]
	default:
		charset = e.charsets[e.gr]
	}
	mapped, ok := charset[c]
	return mapped, ok
}

// wrapGapCell は、右端に入らない幅2の字が次の行へ移ったときに空いた右端のマスである。xterm.js も、そのマスを、
// そのときのペンの何も書いていないマスにする。字の無い幅1のマスとして、字の間の空白と見分ける（isWrapGap）。
func wrapGapCell(pen uv.Style, link uv.Link) uv.Cell {
	return uv.Cell{Width: 1, Style: pen, Link: link}
}

// isWrapGap は、cell が wrapGapCell かを返す。
func isWrapGap(cell *uv.Cell) bool {
	return cell.Content == "" && cell.Width == 1
}
