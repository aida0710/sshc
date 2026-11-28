package vt

import (
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// leavingLinesCapacity は、主画面の scrollback が持つ行の数である。溢れると上流の Scrollback は古い行を黙って
// 捨て、捨てるたびに残りを詰め直す。そこで Write は、出力を、この半分の行数より多くは流せない長さに区切って
// 読み、区切るたびに行を移す（moveLeftLines）。残りの半分は、区切りを字の切れ目まで延ばした分に充てる
// （pieceEnd）。延ばす分は ASCII でないバイトで、1バイトで多くて1行を流す（C1 の IND は1バイトで1行を流す）。
// そこで延ばすのも、この半分のバイト数までにする（pieceExtensionLimit）。
const leavingLinesCapacity = 1 << 16

// pieceExtensionLimit は、pieceEnd が区切りを延ばすバイト数の上限である（leavingLinesCapacity）。
const pieceExtensionLimit = leavingLinesCapacity / 2

// leavingLinesPerByte は、出力の1バイトで主画面から流れうる行の数の上限である。1バイトで終わる操作のうち、
// 多く流すのは画面を消す ED、流す SU・DL、画面より長い REP で、どれも画面の行と、REP が埋めきるのに書く数行
// （repeatSettlingRows と、最後の半端な行と、持っておいた1字の行）より多くは流さない。
func leavingLinesPerByte(height int) int {
	return height + repeatSettlingRows + 2
}

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

// TextLine は、画面の1行の字である（TakeLeftLines、MainScreenLines）。
type TextLine struct {
	// Text は、行の字である。行末の空白は除く。
	Text string
	// WrapGap は、行の最後の桁が、右端に入らない幅2の字が次の行へ移ったために空いた桁（wrapGapCell）かである。
	// その桁は字の間の空白ではない。行は次の行へ続いている。
	WrapGap bool
}

// textLineOf は、width 桁の画面の行 line の字である。scrollback に入った行は、行末の空いたマスを落としてあるので、
// width より短いことがある。
func textLineOf(line uv.Line, width int) TextLine {
	var text strings.Builder
	pendingBlanks := 0
	for index := range line {
		cell := &line[index]
		switch {
		case cell.IsZero():
		case cell.Equal(&uv.EmptyCell) || isWrapGap(cell):
			pendingBlanks++
		default:
			for ; pendingBlanks > 0; pendingBlanks-- {
				text.WriteByte(' ')
			}
			text.WriteString(cell.Content)
		}
	}
	return TextLine{Text: text.String(), WrapGap: width > 0 && len(line) == width && isWrapGap(&line[width-1])}
}

// LeftLine は、主画面から流れた1行である（TakeLeftLines）。
type LeftLine struct {
	TextLine
	// Repeats は、この行のすぐあとに、同じ行がさらに何行流れたかである。画面より長い REP は同じ行を何度も
	// 流すので、エミュレーターはそれを1行ずつ持たずに数える（sshc_repeat.go）。
	Repeats int
}

// TakeLeftLines returns the lines that left the main screen since the last call, oldest first.
func (e *SnapshotEmulator) TakeLeftLines() []LeftLine {
	e.moveLeftLines()
	left := e.left
	e.left = nil
	return left
}

// moveLeftLines は、主画面の scrollback に入った行を、字にして left へ移す。
func (e *SnapshotEmulator) moveLeftLines() {
	kept := e.scrs[0].scrollback
	if kept == nil || len(kept.lines) == 0 {
		return
	}
	for _, line := range kept.lines {
		e.left = append(e.left, LeftLine{TextLine: textLineOf(line, e.Width())})
	}
	kept.Clear()
}

// Write は、エミュレーターの Write である。流れた行を残すエミュレーターは、出力を区切って読み、区切るたびに
// 流れた行を移す。1回の読み取りでも、画面を何度も流す出力（SU を並べる）は、そのバイト数より多くの行を流す。
func (e *SnapshotEmulator) Write(output []byte) (int, error) {
	if !e.leavingLines {
		return e.readOutput(output)
	}
	pieceBytes := max(1, leavingLinesCapacity/2/leavingLinesPerByte(e.Height()))
	for start := 0; start < len(output); {
		end := pieceEnd(output, start+pieceBytes)
		if _, err := e.readOutput(output[start:end]); err != nil {
			return start, err
		}
		e.moveLeftLines()
		start = end
	}
	return len(output), nil
}

// WriteString は Write と同じである。Emulator の WriteString は、区切らない Emulator の Write を呼ぶ。
func (e *SnapshotEmulator) WriteString(output string) (int, error) {
	return e.Write([]byte(output))
}

// pieceEnd は、output を from より後ろで区切る位置を返す。Emulator の Write は、読み終えるときに組み立て中の字を
// 書き出すので、字に続きうるバイト（UTF-8 の2バイト目より後ろや、結合文字）の前では区切らない。ASCII の
// バイトはどれも、前の字に続かない。pieceExtensionLimit バイトのうちに ASCII のバイトが無ければ、そこで区切る。
// パーサーは字の途中の状態を次の Write へ持ち越すので、そのとき割れるのは、区切りをまたぐ字のまとまり（結合文字を
// 付けた字など）だけである。
func pieceEnd(output []byte, from int) int {
	limit := min(len(output), from+pieceExtensionLimit)
	for end := from; end < limit; end++ {
		if output[end] < utf8.RuneSelf {
			return end
		}
	}
	return limit
}

// MainScreenRowsWithText is how many rows from the top of the main screen hold text: the lines that must leave it
// before anything shown after them.
func (e *SnapshotEmulator) MainScreenRowsWithText() int {
	return lastRowWithText(&e.scrs[0]) + 1
}

// MainScreenLines returns the text of the main screen's rows down to the last one with text, without trailing spaces:
// what is still shown when the log ends.
func (e *SnapshotEmulator) MainScreenLines() []TextLine {
	main := &e.scrs[0]
	last := lastRowWithText(main)
	lines := make([]TextLine, 0, last+1)
	for y := 0; y <= last; y++ {
		lines = append(lines, textLineOf(main.buf.Line(y), e.Width()))
	}
	return lines
}
