package vt

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// leavingLinesCapacity bounds the lines kept between two TakeLeftLines calls. The caller takes them after each write
// of at most one read of output, which cannot scroll more lines than it has bytes.
const leavingLinesCapacity = 1 << 16

// NewLeavingLinesEmulator returns an emulator that keeps the lines leaving its main screen, for a log of what a
// terminal showed (the session log, internal/sessionlog). A line leaves when output scrolls it off the top of the
// screen, and when the screen is erased: then the rows down to the last one with text leave, blank rows between them
// included. Lines on the alternate screen never leave into the log: a full-screen program draws there, and the log
// keeps what the shell printed.
func NewLeavingLinesEmulator(width, height int) *SnapshotEmulator {
	e := NewSnapshotEmulator(width, height)
	main := &e.scrs[0]
	main.scrollback = NewScrollback(leavingLinesCapacity)
	e.leavingLines = true
	e.RegisterCsiHandler('J', func(params ansi.Params) bool {
		n, _, _ := params.Param(0, 0)
		switch n {
		case 2:
			if e.scr == main {
				pushRowsWithText(main)
			}
			e.scr.Clear()
			return true
		case 3:
			// Erasing the scrollback leaves nothing on the screen; the lines that left are the log's, not the screen's.
			e.scr.Clear()
			return true
		default:
			return false
		}
	})
	return e
}

// pushRowsWithText puts the rows of screen down to the last one with text into its scrollback.
func pushRowsWithText(screen *Screen) {
	last := lastRowWithText(screen)
	for y := 0; y <= last; y++ {
		screen.scrollback.Push(screen.buf.Line(y))
	}
}

func lastRowWithText(screen *Screen) int {
	for y := screen.buf.Height() - 1; y >= 0; y-- {
		if line := screen.buf.Line(y); line != nil && !screen.isLineEmpty(line) {
			return y
		}
	}
	return -1
}

// keepLeavingLine keeps top, the row scrollUpAtBottom is about to blank, when it leaves the top of the main screen.
func (e *SnapshotEmulator) keepLeavingLine(top uv.Line, regionTop int) {
	if e.leavingLines && e.scr == &e.scrs[0] && regionTop == 0 {
		e.scr.scrollback.Push(top)
	}
}

// TakeLeftLines returns the text of the lines that left the main screen since the last call, oldest first, without
// trailing spaces.
func (e *SnapshotEmulator) TakeLeftLines() []string {
	kept := e.scrs[0].scrollback
	if kept == nil || len(kept.lines) == 0 {
		return nil
	}
	texts := make([]string, len(kept.lines))
	for index, line := range kept.lines {
		texts[index] = line.String()
	}
	kept.Clear()
	return texts
}

// MainScreenRowsWithText is how many rows from the top of the main screen hold text: the lines that must leave it
// before anything shown after them.
func (e *SnapshotEmulator) MainScreenRowsWithText() int {
	return lastRowWithText(&e.scrs[0]) + 1
}

// MainScreenLines returns the text of the main screen's rows down to the last one with text, without trailing spaces:
// what is still shown when the log ends.
func (e *SnapshotEmulator) MainScreenLines() []string {
	main := &e.scrs[0]
	last := lastRowWithText(main)
	texts := make([]string, 0, last+1)
	for y := 0; y <= last; y++ {
		texts = append(texts, main.buf.Line(y).String())
	}
	return texts
}
