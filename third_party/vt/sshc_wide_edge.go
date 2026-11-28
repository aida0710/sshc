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
// ずらし、空いたマスを空白にする。幅2の字はそのままずらす。片方の半分だけが動いたり消えたりした幅2の字は、
// xterm.js（BufferLine の insertCells・deleteCells）と同じに、残った半分を空白にする。カーソルが余白の外にあれば、
// 上流と同じに何もしない。
func (e *SnapshotEmulator) shiftCells(by int) {
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
	count := min(max(by, -by), right-x)
	vacated := row[x : x+count]
	if by > 0 {
		copy(row[x+count:right], row[x:right-count])
	} else {
		copy(row[x:right-count], row[x+count:right])
		vacated = row[right-count : right]
	}
	for column := range vacated {
		vacated[column] = blank
	}
	// 半分を失いうる字は、カーソルの左隣から余白のすぐ外のマス（余白の右端をまたいでいた字の右半分）までにあるが、
	// 行を写すのと同じ手間なので、行のすべてを見る。
	eraseSplitWideCells(row, blank)
}

// eraseSplitWideCells は、行 row のうち、右半分の続かない幅2の字と、幅2の字に続かない右半分（字の無い幅0の
// マス）を blank にする。
func eraseSplitWideCells(row uv.Line, blank uv.Cell) {
	for column := range row {
		cell := &row[column]
		switch {
		case cell.Width > 1 && (column+1 == len(row) || !row[column+1].IsZero()):
			*cell = blank
		case cell.IsZero() && (column == 0 || row[column-1].Width < 2):
			*cell = blank
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
