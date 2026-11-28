package vt

// sshc がこのパッケージの写しに足したもの（README.sshc.md）。字をずらす ICH・DCH と、画面を狭める Resize で、幅2の
// 字を壊さない。上流の ICH は、カーソルの上の幅2の字を右へずらしたあと、空けるマスを書くときにずらした先の字まで
// 空白にする。上流の DCH は、字を1マスずつ書いて左へずらすので、幅2の字の右半分を書くときにその字を空白にする。
// どちらも、片方の半分を失った幅2の字を行に残すこともある。Resize は、右半分を切った字の左半分を最後の桁に
// 残す。スナップショットがその字を最後の桁に書くと、ブラウザのターミナルは次の行へ折り返し、下の行なら画面を
// 流す。

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// keepWideCharactersWholeWhenShifting は、ICH（CSI Ps @）と DCH（CSI Ps P）を、shiftCells で扱う。
func (e *SnapshotEmulator) keepWideCharactersWholeWhenShifting() {
	e.RegisterCsiHandler('@', func(params ansi.Params) bool {
		count, _, _ := params.Param(0, 1)
		e.shiftCells(count)
		return true
	})
	e.RegisterCsiHandler('P', func(params ansi.Params) bool {
		count, _, _ := params.Param(0, 1)
		e.shiftCells(-count)
		return true
	})
}

// shiftCells は、カーソルから余白の右端までのマスを、by が正なら by 桁右へ（ICH）、負なら -by 桁左へ（DCH）
// ずらし、空いたマスを空白にする。幅2の字はそのままずらす。カーソルが余白の外にあれば、上流と同じに字は動かさない。
func (e *SnapshotEmulator) shiftCells(by int) {
	// xterm.js は、ICH・DCH の前にカーソルを右端のマスへ戻すので、右端での折り返しの待ちはここで終わる。
	e.atPhantom = false
	screen := e.scr
	x, y := screen.CursorPosition()
	region := screen.scroll
	if by == 0 || !uv.Pos(x, y).In(region) || y >= len(screen.buf.Lines) {
		return
	}
	row := screen.buf.Lines[y]
	right := min(region.Max.X, len(row))
	if x >= right {
		return
	}
	blank := uv.EmptyCell
	if cell := screen.blankCell(); cell != nil {
		blank = *cell
	}
	shift := cellShift{row: row, from: x, to: right, count: min(max(by, -by), right-x), blank: blank}
	if by > 0 {
		shift.insert()
	} else {
		shift.delete()
	}
}

// cellShift は、行 row の from から to の手前までのマスを count 桁ずらすことである。片方の半分を失う幅2の字は、
// xterm.js（BufferLine の insertCells・deleteCells）と同じに、残った半分を blank にする。
type cellShift struct {
	row      uv.Line
	from, to int
	count    int
	blank    uv.Cell
}

// insert は、マスを count 桁右へずらし、from から count 桁を空ける（ICH）。
func (s cellShift) insert() {
	row := s.row
	// カーソルが幅2の字の右半分にあれば、その右半分は右へずれて残る。xterm.js はその半分を字の属性のまま幅0の
	// マスとして残し、WebGL の描画はそのマスに字の背景色を塗るので、その背景色の空白にする。
	split := s.from > 0 && row[s.from-1].Width > 1
	var splitBackground uv.Style
	if split {
		splitBackground = uv.Style{Bg: row[s.from-1].Style.Bg}
	}
	copy(row[s.from+s.count:s.to], row[s.from:s.to-s.count])
	s.fill(s.from, s.from+s.count)
	if moved := s.from + s.count; split && moved < s.to {
		row[moved] = uv.Cell{Content: " ", Width: 1, Style: splitBackground}
	}
	// 余白の最後のマスに来た左半分は、右半分が余白の外へ出て消えた。余白のすぐ外に右半分が残っていても、それは
	// 前にそこにあった別の字の半分である。
	if row[s.to-1].Width > 1 {
		row[s.to-1] = s.blank
	}
	s.eraseSplitWideCells()
}

// delete は、from から count 桁を消し、残りのマスを左へ寄せる（DCH）。
func (s cellShift) delete() {
	row := s.row
	// カーソルが幅2の字の右半分にあれば、残った左半分を空ける。カーソルへ寄せたマスが右半分でも、それは消した
	// 別の字の半分である。
	if s.from > 0 && row[s.from-1].Width > 1 {
		row[s.from-1] = s.blank
	}
	copy(row[s.from:s.to-s.count], row[s.from+s.count:s.to])
	s.fill(s.to-s.count, s.to)
	s.eraseSplitWideCells()
}

// fill は、行の from から to の手前までのマスを blank にする。
func (s cellShift) fill(from, to int) {
	for column := from; column < to; column++ {
		s.row[column] = s.blank
	}
}

// eraseSplitWideCells は、カーソルの左隣から余白のすぐ外までのマスのうち、右半分の続かない幅2の字と、幅2の字に
// 続かない右半分（字の無い幅0のマス）を blank にする。字をずらして半分を失いうるのは、この範囲の字だけである。
func (s cellShift) eraseSplitWideCells() {
	row := s.row
	for column := max(s.from-1, 0); column < min(s.to+1, len(row)); column++ {
		cell := &row[column]
		switch {
		case cell.Width > 1 && (column+1 == len(row) || !row[column+1].IsZero()):
			*cell = s.blank
		case cell.IsZero() && (column == 0 || row[column-1].Width < 2):
			*cell = s.blank
		}
	}
}

// eraseWideCellsCutAtTheRightEdge は、画面を狭めて右半分を切られた幅2の字を、空いたマスにする。
func eraseWideCellsCutAtTheRightEdge(screen *Screen) {
	for _, row := range screen.buf.Lines {
		if last := len(row) - 1; last >= 0 && row[last].Width > 1 {
			row[last] = uv.EmptyCell
		}
	}
}
