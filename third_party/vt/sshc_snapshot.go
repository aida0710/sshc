package vt

// sshc's addition to its copy of the package (README.sshc.md). The sshc engine keeps an Emulator for a terminal
// session's output and writes its state out for a browser terminal that cannot replay the output itself.

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/parser"
)

// kittyStackLimit is how many kitty keyboard flags xterm.js keeps stacked for each screen; it drops the oldest past it.
const kittyStackLimit = 16

// controlStringLimit is how much of a control string (OSC, DCS and the like) the emulator keeps. A snapshot needs
// only the short ones (a link, a title); the upstream emulator sets aside 4 MiB, for each session, for images it
// does not draw. A longer string is still read to its end.
const controlStringLimit = 16 << 10

// replayedDECModes are the DEC private modes a snapshot carries over, for programs that set them once when they
// start: the keys, the mouse reports and their encodings, focus reports, bracketed paste, grapheme clusters (2027),
// colour scheme updates (2031) and paste notifications (5522). The screens (47, 1047, 1048, 1049), the cursor
// (25), origin mode (6) and wraparound (7) are written out on their own; synchronized output (2026) never is, so
// the terminal draws what follows.
var replayedDECModes = []int{1, 5, 9, 66, 1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1015, 1016, 2004, 2027, 2031, 5522}

// replayedANSIModes are the ANSI modes a snapshot carries over: insert (4) and automatic newline (20).
var replayedANSIModes = []int{4, 20}

// kittyKeyboard follows the kitty keyboard flags the way xterm.js keeps them: the flags in use, the flags kept for
// the screen not shown, and a stack for each screen.
type kittyKeyboard struct {
	flags               int
	mainFlags, altFlags int
	mainStack, altStack []int
}

// SnapshotEmulator is an Emulator whose state can be written out with Snapshot.
type SnapshotEmulator struct {
	*Emulator
	kitty kittyKeyboard
	// autowrap follows the wraparound mode (?7), which every printed character reads.
	autowrap bool
	// blankRow is a row of the cell a scroll fills the new bottom row with, kept to copy from.
	blankRow uv.Line
}

// asciiContents are the cell contents of the printable ASCII characters, made once: the emulator's own print makes
// a new string for every character.
var asciiContents = func() (contents [ansi.DEL]string) {
	for c := byte(ansi.SP); c < ansi.DEL; c++ {
		contents[c] = string(rune(c))
	}
	return contents
}()

// NewSnapshotEmulator returns an emulator for following a terminal's output. It keeps no scrollback: the output
// the engine still holds is replayed after a snapshot, and older lines are not kept. It sends no replies: the
// terminal the program talks to answers it.
func NewSnapshotEmulator(w, h int) *SnapshotEmulator {
	e := &SnapshotEmulator{Emulator: NewEmulator(w, h)}
	e.parser.SetDataSize(controlStringLimit)
	e.scrs[0].scrollback = nil
	e.scrs[1].scrollback = nil
	// Replies go into a pipe that nothing reads, so writing one would block. Closing the reading end makes those
	// writes fail at once; the emulator ignores their errors.
	_ = e.pr.Close()
	e.followKittyKeyboard()
	e.autowrap = e.isModeSet(ansi.ModeAutoWrap)
	e.cb.EnableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeAutoWrap {
			e.autowrap = true
		}
	}
	e.cb.DisableMode = func(mode ansi.Mode) {
		if mode == ansi.ModeAutoWrap {
			e.autowrap = false
		}
	}
	// Handlers registered later run first: this one takes a line feed that scrolls, and leaves the others to the
	// emulator's own.
	for _, code := range []byte{ansi.LF, ansi.VT, ansi.FF} {
		e.registerCcHandler(code, e.scrollingLinefeed)
	}
	e.parser.SetHandler(ansi.Handler{
		Print:     e.print,
		Execute:   e.handleControl,
		HandleCsi: e.handleCsi,
		HandleEsc: e.handleEsc,
		HandleDcs: e.handleDcs,
		HandleOsc: e.handleOsc,
		HandleApc: e.handleApc,
		HandlePm:  e.handlePm,
		HandleSos: e.handleSos,
	})
	return e
}

// print is the emulator's print with a shorter way for the common case. The emulator holds each character back
// until the next one shows its grapheme cluster has ended; a printable ASCII character after a printable ASCII one
// ends it for certain, and the one held back needs neither the segmenter nor a new string. Any escape sequence or
// control character puts out the one held back first, so the character sets in use are still the ones for it.
func (e *SnapshotEmulator) print(r rune) {
	if r >= ansi.SP && r < ansi.DEL && len(e.grapheme) == 1 && e.grapheme[0] >= ansi.SP && e.grapheme[0] < ansi.DEL &&
		e.charsets[e.gl] == nil && e.gsingle == 0 {
		e.putASCII(e.grapheme[0])
		e.grapheme[0] = byte(r)
		return
	}
	e.handlePrint(r)
}

// putASCII is the emulator's handleGrapheme for one printable ASCII character with no character set to map it.
func (e *SnapshotEmulator) putASCII(c byte) {
	screen := e.scr
	x, y := screen.cur.X, screen.cur.Y
	if e.atPhantom && e.autowrap {
		if !e.scrollUpAtBottom() {
			e.index()
		}
		y, x = screen.cur.Y, 0
	}
	e.lastChar = rune(c)
	cell := uv.Cell{Content: asciiContents[c], Width: 1, Style: screen.cur.Pen, Link: screen.cur.Link}
	// Over a one-column cell there is no wide character to clear first, which is all the buffer's SetCell adds
	// besides marking the row for redrawing, which nothing here does.
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

// scrollingLinefeed is the emulator's line feed when it scrolls (scrollUpAtBottom). It reports false to leave
// every other line feed to the emulator.
func (e *SnapshotEmulator) scrollingLinefeed() bool {
	if !e.scrollUpAtBottom() {
		return false
	}
	if e.isModeSet(ansi.ModeLineFeedNewLine) {
		e.carriageReturn()
	}
	return true
}

// scrollUpAtBottom is the emulator's index on the bottom row of a scroll region as wide as the screen, the one
// output scrolls by. The emulator's own moves every cell of the region up one by one, which made following output
// with it about a hundred times slower than writing it to the replay buffer; this moves the rows instead. It
// reports false, doing nothing, anywhere else.
func (e *SnapshotEmulator) scrollUpAtBottom() bool {
	screen := e.scr
	x, y := screen.CursorPosition()
	region := screen.scroll
	if y != region.Max.Y-1 || x < region.Min.X || x >= region.Max.X || region.Min.X != 0 || region.Max.X != screen.Width() {
		return false
	}
	rows := screen.buf.Lines
	top := rows[region.Min.Y]
	copy(rows[region.Min.Y:region.Max.Y-1], rows[region.Min.Y+1:region.Max.Y])
	blank := uv.EmptyCell
	if cell := screen.blankCell(); cell != nil {
		blank = *cell
	}
	if len(e.blankRow) != len(top) || !e.blankRow[0].Equal(&blank) {
		e.blankRow = make(uv.Line, len(top))
		for index := range e.blankRow {
			e.blankRow[index] = blank
		}
	}
	copy(top, e.blankRow)
	rows[region.Max.Y-1] = top
	e.atPhantom = false
	return true
}

// AtGround reports whether the emulator has taken every sequence it was given: output read so far does not end
// halfway through an escape sequence or a UTF-8 character.
func (e *SnapshotEmulator) AtGround() bool {
	return e.parser.State() == parser.GroundState
}

func (e *SnapshotEmulator) altScreenShown() bool { return e.scr == &e.scrs[1] }

func (e *SnapshotEmulator) followKittyKeyboard() {
	k := &e.kitty
	stack := func() *[]int {
		if e.altScreenShown() {
			return &k.altStack
		}
		return &k.mainStack
	}
	e.RegisterCsiHandler(ansi.Command('=', 0, 'u'), func(params ansi.Params) bool {
		flags, _, _ := params.Param(0, 0)
		mode, _, _ := params.Param(1, 1)
		switch mode {
		case 1:
			k.flags = flags
		case 2:
			k.flags |= flags
		case 3:
			k.flags &^= flags
		}
		return true
	})
	e.RegisterCsiHandler(ansi.Command('>', 0, 'u'), func(params ansi.Params) bool {
		flags, _, _ := params.Param(0, 0)
		s := stack()
		if len(*s) >= kittyStackLimit {
			*s = (*s)[1:]
		}
		*s = append(*s, k.flags)
		k.flags = flags
		return true
	})
	e.RegisterCsiHandler(ansi.Command('<', 0, 'u'), func(params ansi.Params) bool {
		count, _, _ := params.Param(0, 1)
		count = max(1, count)
		s := stack()
		for ; count > 0 && len(*s) > 0; count-- {
			k.flags = (*s)[len(*s)-1]
			*s = (*s)[:len(*s)-1]
		}
		if len(*s) == 0 {
			k.flags = 0
		}
		return true
	})
	// The screens keep their flags apart; a reset forgets them all.
	shownBefore := e.altScreenShown()
	e.cb.AltScreen = func(on bool) {
		if on == shownBefore {
			return
		}
		shownBefore = on
		if on {
			k.mainFlags, k.flags = k.flags, k.altFlags
		} else {
			k.altFlags, k.flags = k.flags, k.mainFlags
		}
	}
	// RIS is the emulator's own. Handlers registered later run first, so this one runs before it and leaves the
	// sequence to it.
	e.RegisterEscHandler('c', func() bool {
		e.kitty = kittyKeyboard{}
		return false
	})
	e.RegisterCsiHandler(ansi.Command(0, '!', 'p'), func(ansi.Params) bool {
		e.softReset()
		return true
	})
}

// softReset is DECSTR as xterm.js does it: the upstream emulator does not take it, and sshc ends every program's
// output with one (internal/terminal/leftover_modes.go).
func (e *SnapshotEmulator) softReset() {
	screen := e.scr
	screen.setCursorHidden(false)
	screen.scroll = screen.buf.Bounds()
	screen.cur.Pen = uv.Style{}
	screen.cur.Link = uv.Link{}
	screen.saved = Cursor{}
	for _, mode := range []ansi.Mode{ansi.ModeCursorKeys, ansi.ModeOrigin, ansi.ModeNumericKeypad, ansi.ModeFocusEvent,
		ansi.ModeBracketedPaste, ansi.DECMode(45), ansi.DECMode(2026), ansi.DECMode(2031), ansi.ANSIMode(4), ansi.ANSIMode(20)} {
		if e.isModeSet(mode) {
			e.setMode(mode, ansi.ModeReset)
		}
	}
	e.setMode(ansi.ModeAutoWrap, ansi.ModeSet)
	e.charsets = [4]CharSet{}
	e.gl, e.gr, e.gsingle = 0, 1, 0
	e.kitty = kittyKeyboard{}
}

// Snapshot returns the sequences that bring a terminal to the emulator's state: both screens, where each keeps its
// cursor, the scroll region, the modes, the character sets and the pen. It works on a terminal that showed
// something before, without clearing that terminal's scrollback.
func (e *SnapshotEmulator) Snapshot() string {
	e.flushGrapheme()
	var b strings.Builder
	e.writePrologue(&b)
	main, alt := &e.scrs[0], &e.scrs[1]
	e.writeScreen(&b, main)
	if e.altScreenShown() {
		// The cursor at the switch is the one the normal screen gets back when the program leaves (?1049l).
		writeCursorPosition(&b, main.cur.Position)
		writeKittyKeyboard(&b, e.kitty.mainStack, e.kitty.mainFlags)
		b.WriteString(ansi.SetModeAltScreenSaveCursor)
		b.WriteString(ansi.PopKittyKeyboard(99))
		e.writeScreen(&b, alt)
		writeKittyKeyboard(&b, e.kitty.altStack, e.kitty.flags)
	} else {
		writeKittyKeyboard(&b, e.kitty.mainStack, e.kitty.flags)
	}
	e.writeState(&b, e.scr)
	return b.String()
}

// writePrologue leaves a terminal that showed something before as a fresh one would be, except for its scrollback.
func (e *SnapshotEmulator) writePrologue(b *strings.Builder) {
	b.WriteString(ansi.ResetModeAltScreenSaveCursor)
	b.WriteString("\x1b[!p") // DECSTR: the pen, the margins, the cursor, the character sets and the kitty flags
	b.WriteString(ansi.PopKittyKeyboard(99))
	for _, mode := range replayedDECModes {
		b.WriteString(ansi.ResetMode(ansi.DECMode(mode)))
	}
	for _, mode := range replayedANSIModes {
		b.WriteString(ansi.ResetMode(ansi.ANSIMode(mode)))
	}
	b.WriteString(ansi.ResetHyperlink())
	b.WriteString(ansi.CursorHomePosition)
	b.WriteString(ansi.EraseEntireScreen)
}

// writeScreen draws every row of screen at its place, then the screen's scroll region and the cursor it saved.
func (e *SnapshotEmulator) writeScreen(b *strings.Builder, screen *Screen) {
	for y := range screen.Height() {
		var row strings.Builder
		writeRow(&row, screen.buf.Line(y))
		if row.Len() > 0 {
			b.WriteString(ansi.CursorPosition(1, y+1))
			b.WriteString(row.String())
		}
	}
	if region := screen.scroll; region != screen.buf.Bounds() {
		b.WriteString(ansi.SetTopBottomMargins(region.Min.Y+1, region.Max.Y))
	}
	if saved := screen.saved; saved != (Cursor{}) {
		writeCursorPosition(b, saved.Position)
		b.WriteString(saved.Pen.String())
		b.WriteString(ansi.SaveCursor)
		b.WriteString(ansi.ResetStyle)
	}
}

// writeRow writes one row from its first column. A cell wider than one column is followed by the column the next
// cell starts at, so a terminal that measures the character otherwise still puts the rest of the row in place.
// Blank cells with a colour are erased in that colour rather than written as spaces, as programs leave them: a
// copy of the row then ends where its text does.
func writeRow(b *strings.Builder, line uv.Line) {
	var pen uv.Style
	var link uv.Link
	setPen := func(style uv.Style) {
		if style.Equal(&pen) {
			return
		}
		b.WriteString(ansi.ResetStyle)
		if !style.IsZero() {
			b.WriteString(style.String())
		}
		pen = style
	}
	setLink := func(to uv.Link) {
		if to == link {
			return
		}
		if to.URL == "" {
			b.WriteString(ansi.ResetHyperlink())
		} else {
			b.WriteString(ansi.SetHyperlink(to.URL, to.Params))
		}
		link = to
	}
	pendingBlanks := 0
	for x := 0; x < len(line); x++ {
		cell := &line[x]
		if cell.IsZero() {
			continue
		}
		if cell.Equal(&uv.EmptyCell) {
			pendingBlanks++
			continue
		}
		if pendingBlanks > 0 {
			setPen(uv.Style{})
			setLink(uv.Link{})
			b.WriteString(strings.Repeat(" ", pendingBlanks))
			pendingBlanks = 0
		}
		if run := colouredBlanks(line[x:], cell.Style); run > 0 {
			setPen(cell.Style)
			setLink(uv.Link{})
			b.WriteString(ansi.EraseCharacter(run))
			x += run - 1
			if x+1 < len(line) {
				b.WriteString(ansi.CursorHorizontalAbsolute(x + 2))
			}
			continue
		}
		setPen(cell.Style)
		setLink(cell.Link)
		b.WriteString(cell.Content)
		if cell.Width > 1 && x+cell.Width < len(line) {
			b.WriteString(ansi.CursorHorizontalAbsolute(x + cell.Width + 1))
		}
	}
	setLink(uv.Link{})
	setPen(uv.Style{})
}

// colouredBlanks counts the blank cells with style and no link at the start of cells, when style has a colour
// that erasing keeps (the background).
func colouredBlanks(cells []uv.Cell, style uv.Style) int {
	if style.Bg == nil {
		return 0
	}
	count := 0
	for count < len(cells) {
		cell := &cells[count]
		if cell.Content != " " || cell.Width != 1 || cell.Link != (uv.Link{}) || !cell.Style.Equal(&style) {
			break
		}
		count++
	}
	return count
}

// writeState sets what applies from here on to the screen shown: the character sets, the modes, the cursor and
// the pen.
func (e *SnapshotEmulator) writeState(b *strings.Builder, screen *Screen) {
	for g, charset := range e.charsets {
		if designator := charsetDesignator(charset); designator != 0 {
			b.WriteString("\x1b" + string("()*+"[g]) + string(designator))
		}
	}
	if e.gl == 1 {
		b.WriteByte(ansi.SO)
	}
	for _, mode := range replayedDECModes {
		if e.isModeSet(ansi.DECMode(mode)) {
			b.WriteString(ansi.SetMode(ansi.DECMode(mode)))
		}
	}
	for _, mode := range replayedANSIModes {
		if e.isModeSet(ansi.ANSIMode(mode)) {
			b.WriteString(ansi.SetMode(ansi.ANSIMode(mode)))
		}
	}
	cursor := screen.cur
	if cursor.Hidden {
		b.WriteString(ansi.ResetModeTextCursorEnable)
	}
	if cursor.Style != CursorBlock || cursor.Steady {
		b.WriteString(ansi.SetCursorStyle(cursorStyleNumber(cursor)))
	}
	position := cursor.Position
	if e.atPhantom {
		// The cursor waits past the last column: the next character starts a new row. Writing the last cell again
		// leaves a terminal waiting the same way.
		last := screen.buf.Line(position.Y).At(screen.Width() - 1)
		if last != nil && !last.IsZero() && last.Width == 1 {
			writeCursorPosition(b, uv.Pos(screen.Width()-1, position.Y))
			b.WriteString(last.Style.String())
			b.WriteString(last.Content)
			b.WriteString(ansi.ResetStyle)
			position = uv.Position{X: -1}
		}
	}
	if !e.isModeSet(ansi.ModeAutoWrap) {
		b.WriteString(ansi.ResetModeAutoWrap)
	}
	if e.isModeSet(ansi.ModeOrigin) {
		// Origin mode puts the cursor at the top of the scroll region; positions count from there.
		b.WriteString(ansi.SetModeOrigin)
		position.Y -= screen.scroll.Min.Y
	}
	if position.X >= 0 {
		writeCursorPosition(b, position)
	}
	if !cursor.Pen.IsZero() {
		b.WriteString(cursor.Pen.String())
	}
	if cursor.Link.URL != "" {
		b.WriteString(ansi.SetHyperlink(cursor.Link.URL, cursor.Link.Params))
	}
}

func writeCursorPosition(b *strings.Builder, position uv.Position) {
	b.WriteString(ansi.CursorPosition(position.X+1, position.Y+1))
}

// writeKittyKeyboard rebuilds a screen's kitty keyboard flags on a terminal whose stack is empty: the first entry
// is set, the others and the flags in use are pushed over it.
func writeKittyKeyboard(b *strings.Builder, stack []int, flags int) {
	if len(stack) == 0 {
		if flags != 0 {
			b.WriteString(ansi.KittyKeyboard(flags, 1))
		}
		return
	}
	b.WriteString(ansi.KittyKeyboard(stack[0], 1))
	for _, entry := range stack[1:] {
		b.WriteString(ansi.PushKittyKeyboard(entry))
	}
	b.WriteString(ansi.PushKittyKeyboard(flags))
}

func charsetDesignator(charset CharSet) byte {
	switch {
	case charset == nil:
		return 0
	case len(charset) == len(SpecialDrawing) && charset['q'] == SpecialDrawing['q']:
		return '0'
	case len(charset) == len(UK) && charset['$'] == UK['$']:
		return 'A'
	}
	return 0
}

// cursorStyleNumber is the DECSCUSR number of a cursor: 1 and 2 are a block, 3 and 4 an underline, 5 and 6 a bar;
// the even ones do not blink.
func cursorStyleNumber(cursor Cursor) int {
	number := 1 + 2*int(cursor.Style)
	if cursor.Steady {
		number++
	}
	return number
}
