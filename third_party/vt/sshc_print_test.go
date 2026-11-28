package vt

import (
	"slices"
	"strings"
	"testing"
)

// shownRows は、画面の各行の字である。行末の空いたマスは除く。
func shownRows(e *SnapshotEmulator) []string {
	var rows []string
	for _, cells := range e.ShownCells() {
		rows = append(rows, strings.TrimRight(strings.Join(cells, ""), " "))
	}
	return rows
}

// shownScreen は、画面の各行の字（行末の空いたマスは除く）と、カーソルの位置である。
type shownScreen struct {
	rows             []string
	cursorX, cursorY int
}

func checkScreen(t *testing.T, e *SnapshotEmulator, want shownScreen) {
	t.Helper()
	if got := shownRows(e); !slices.Equal(got, want.rows) {
		t.Fatalf("screen = %q, want %q", got, want.rows)
	}
	if x, y := e.scr.CursorPosition(); x != want.cursorX || y != want.cursorY {
		t.Fatalf("cursor at (%d, %d), want (%d, %d)", x, y, want.cursorX, want.cursorY)
	}
}

// 右端の1桁に入らない幅2の字は、xterm.js と同じく、右端を空けて次の行の頭に書く。
func TestAWideCharacterThatDoesNotFitInTheLastColumnGoesToTheNextRow(t *testing.T) {
	for name, test := range map[string]struct {
		height int
		output string
		want   shownScreen
	}{
		"the second of two wide characters after 37 columns": {
			height: 3,
			output: strings.Repeat("a", 37) + "日本",
			want:   shownScreen{rows: []string{strings.Repeat("a", 37) + "日", "本", ""}, cursorX: 2, cursorY: 1},
		},
		"a wide character after 39 columns": {
			height: 3,
			output: strings.Repeat("a", 39) + "日x",
			want:   shownScreen{rows: []string{strings.Repeat("a", 39), "日x", ""}, cursorX: 3, cursorY: 1},
		},
		"on the bottom row the screen scrolls": {
			height: 2,
			output: "top\r\n" + strings.Repeat("a", 39) + "日",
			want:   shownScreen{rows: []string{strings.Repeat("a", 39), "日"}, cursorX: 2, cursorY: 1},
		},
		"in a coloured pen": {
			height: 3,
			output: "\x1b[41m" + strings.Repeat("a", 39) + "日\x1b[0m",
			want:   shownScreen{rows: []string{strings.Repeat("a", 39), "日", ""}, cursorX: 2, cursorY: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, test.height)
			writeAll(t, e, test.output)
			checkScreen(t, e, test.want)
		})
	}
}

// 右端の2桁をちょうど埋めた幅2の字のあとは、次の字を次の行に書く。次の字でその字を消さない。
func TestAWideCharacterThatFillsTheLastTwoColumnsWaitsToWrap(t *testing.T) {
	full := strings.Repeat("a", 38) + "日"
	for name, test := range map[string]struct {
		after string
		want  shownScreen
	}{
		"a narrow character comes next": {after: "x", want: shownScreen{rows: []string{full, "x", ""}, cursorX: 1, cursorY: 1}},
		"a wide character comes next":   {after: "本", want: shownScreen{rows: []string{full, "本", ""}, cursorX: 2, cursorY: 1}},
		"a line break comes next":       {after: "\r\nx", want: shownScreen{rows: []string{full, "x", ""}, cursorX: 1, cursorY: 1}},
		"the cursor moves before the next": {
			after: "\x1b[1;1Hx",
			want:  shownScreen{rows: []string{"x" + full[1:], "", ""}, cursorX: 1, cursorY: 0},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, 3)
			writeAll(t, e, full+test.after)
			checkScreen(t, e, test.want)
		})
	}
}

// 折り返し（DECAWM）を切っていると、xterm.js は右端に入らない幅2の字を書かずに捨て、カーソルを右端に置く。
// 右端にあった字は残る。
func TestWithWraparoundOffAWideCharacterThatDoesNotFitIsLeftOut(t *testing.T) {
	e := NewSnapshotEmulator(40, 2)
	lastColumn := 39
	writeAll(t, e, "\x1b[?7l"+strings.Repeat("a", 40)+"日")
	checkScreen(t, e, shownScreen{rows: []string{strings.Repeat("a", 40), ""}, cursorX: lastColumn})
	writeAll(t, e, "Z")
	checkScreen(t, e, shownScreen{rows: []string{strings.Repeat("a", 39) + "Z", ""}, cursorX: lastColumn})

	// 右端まで書いて折り返しを待っていても、入らない字を捨てたあとは待たない。折り返しを戻しても、次の字は右端に書く。
	waiting := NewSnapshotEmulator(40, 2)
	writeAll(t, waiting, strings.Repeat("a", 40)+"\x1b[?7l日\x1b[?7hZ")
	checkScreen(t, waiting, shownScreen{rows: []string{strings.Repeat("a", 39) + "Z", ""}, cursorX: lastColumn})
}

// 字は、続く字が結合文字でないと分かるまで持っておく。何が続いても、持っておいた幅2の字は次の行に書く。
func TestAWideCharacterHeldBackIsWrappedWhateverComesNext(t *testing.T) {
	before := strings.Repeat("a", 37) + "日本"
	first := strings.Repeat("a", 37) + "日"
	for name, test := range map[string]struct {
		after string
		rows  []string
	}{
		"a narrow character":       {after: "x", rows: []string{first, "本x", ""}},
		"another wide character":   {after: "語", rows: []string{first, "本語", ""}},
		"a combining mark":         {after: "́", rows: []string{first, "本́", ""}},
		"a carriage return and LF": {after: "\r\nx", rows: []string{first, "本", "x"}},
		"a CSI sequence":           {after: "\x1b[1mx", rows: []string{first, "本x", ""}},
		"an ESC sequence":          {after: "\x1b7x", rows: []string{first, "本x", ""}},
		"an OSC sequence":          {after: "\x1b]0;r100\x07x", rows: []string{first, "本x", ""}},
		"a DCS sequence":           {after: "\x1bP$q\"p\x1b\\x", rows: []string{first, "本x", ""}},
		"an APC sequence":          {after: "\x1b_r100\x1b\\x", rows: []string{first, "本x", ""}},
		"a PM sequence":            {after: "\x1b^r100\x1b\\x", rows: []string{first, "本x", ""}},
		"a SOS sequence":           {after: "\x1bXr100\x1b\\x", rows: []string{first, "本x", ""}},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, 3)
			writeAll(t, e, before+test.after)
			if got := shownRows(e); !slices.Equal(got, test.rows) {
				t.Fatalf("screen = %q, want %q", got, test.rows)
			}
		})
	}
}

// 出力の終わりに持っておいた字も、読み終えたときに次の行へ書く。字のバイトが2回の読み取りに分かれても同じである。
func TestAWideCharacterAtTheEndOfAReadIsWrapped(t *testing.T) {
	first := strings.Repeat("a", 37) + "日"
	for name, newEmulator := range map[string]func(int, int) *SnapshotEmulator{
		"the snapshot emulator":      NewSnapshotEmulator,
		"the leaving lines emulator": NewLeavingLinesEmulator,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEmulator(40, 3)
			writeAll(t, e, first+"本")
			// CellAt は持っておいた字を書き出さないので、読み取りが書いたものを見る。
			if cell := e.CellAt(0, 1); cell == nil || cell.Content != "本" {
				t.Fatalf("after one read the next row starts with %+v, want 本", cell)
			}
			split := newEmulator(40, 3)
			wide := []byte("本")
			writeAll(t, split, first+string(wide[:1]))
			writeAll(t, split, string(wide[1:]))
			if cell := split.CellAt(0, 1); cell == nil || cell.Content != "本" {
				t.Fatalf("after a read split inside the character the next row starts with %+v, want 本", cell)
			}
		})
	}
}

// 字を書いた画面を流れた行は、右端の空いたマスを字の間の空白と見ない。
func TestARowLeftByAWideCharacterThatWrappedSaysItsLastColumnIsNotText(t *testing.T) {
	e := NewLeavingLinesEmulator(40, 2)
	writeAll(t, e, strings.Repeat("a", 39)+"日本\r\n"+strings.Repeat("b", 38)+"語\r\n\r\n")
	left := e.TakeLeftLines()
	want := []TextLine{
		{Text: strings.Repeat("a", 39), WrapGap: true},
		{Text: "日本"},
		{Text: strings.Repeat("b", 38) + "語"},
	}
	var got []TextLine
	for _, line := range left {
		got = append(got, line.TextLine)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("left = %+v, want %+v", got, want)
	}
}

// 幅2の字が次の行へ移って空いた右端の桁は、ほかの空いたマスと同じに " " である。"" は幅2の字の右半分を表す。
func TestTheColumnAWideCharacterLeftEmptyShowsAsAnEmptyCell(t *testing.T) {
	e := NewSnapshotEmulator(40, 2)
	writeAll(t, e, strings.Repeat("a", 39)+"日")
	if got := e.ShownCells()[0][39]; got != " " {
		t.Fatalf("the last column shows %q, want an empty cell", got)
	}
}
