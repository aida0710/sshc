package vt

import (
	"strings"
	"testing"
)

// 右端で折り返しを待っているときに、ICH・DCH・IL・DL・RI・カーソルの復元・代替画面から戻る ?1049l を受けると、
// xterm.js と同じに待ちをやめる。次の字は、次の行ではなく、その操作のあとのカーソルの位置に書く。右端の2桁を
// 埋めた幅2の字のあとも、右端の1桁を埋めた字のあとも同じである。
func TestTheWaitToWrapEndsAtShiftsLineMovesAndCursorRestores(t *testing.T) {
	const lastColumn = 39
	a := func(count int) string { return strings.Repeat("a", count) }
	wide, narrow := a(38)+"日", a(40)
	for name, test := range map[string]struct {
		height int
		output string
		want   shownScreen
	}{
		"DCH after a wide character": {
			height: 3, output: wide + "\x1b[Px",
			want: shownScreen{rows: []string{a(38) + " x", "", ""}, cursorX: lastColumn},
		},
		"DCH after a narrow character": {
			height: 3, output: narrow + "\x1b[Px",
			want: shownScreen{rows: []string{a(39) + "x", "", ""}, cursorX: lastColumn},
		},
		"ICH after a wide character": {
			height: 3, output: wide + "\x1b[@x",
			want: shownScreen{rows: []string{a(38) + " x", "", ""}, cursorX: lastColumn},
		},
		"ICH outside the scroll region after a wide character": {
			height: 4, output: "\x1b[1;2r\x1b[4;1H" + wide + "\x1b[@x",
			want: shownScreen{rows: []string{"", "", "", a(38) + " x"}, cursorX: lastColumn, cursorY: 3},
		},
		"IL after a wide character": {
			height: 3, output: wide + "\x1b[Lx",
			want: shownScreen{rows: []string{"x", wide, ""}, cursorX: 1},
		},
		"IL after a narrow character": {
			height: 3, output: narrow + "\x1b[Lx",
			want: shownScreen{rows: []string{"x", narrow, ""}, cursorX: 1},
		},
		"IL outside the scroll region after a wide character": {
			height: 4, output: "\x1b[1;2r\x1b[4;1H" + wide + "\x1b[Lx",
			want: shownScreen{rows: []string{"", "", "", a(38) + " x"}, cursorX: lastColumn, cursorY: 3},
		},
		"DL after a wide character": {
			height: 3, output: wide + "\x1b[Mx",
			want: shownScreen{rows: []string{"x", "", ""}, cursorX: 1},
		},
		"RI after a wide character": {
			height: 3, output: "\r\n" + wide + "\x1bMx",
			want: shownScreen{rows: []string{strings.Repeat(" ", 39) + "x", wide, ""}, cursorX: lastColumn},
		},
		"RI after a narrow character": {
			height: 3, output: "\r\n" + narrow + "\x1bMx",
			want: shownScreen{rows: []string{strings.Repeat(" ", 39) + "x", narrow, ""}, cursorX: lastColumn},
		},
		"DECSC and DECRC after a wide character": {
			height: 3, output: wide + "\x1b7\x1b8x",
			want: shownScreen{rows: []string{a(38) + " x", "", ""}, cursorX: lastColumn},
		},
		"DECSC and DECRC after a narrow character": {
			height: 3, output: narrow + "\x1b7\x1b8x",
			want: shownScreen{rows: []string{a(39) + "x", "", ""}, cursorX: lastColumn},
		},
		"DECRC of the cursor saved at the top left": {
			height: 3, output: wide + "\x1b8x",
			want: shownScreen{rows: []string{"x" + a(37) + "日", "", ""}, cursorX: 1},
		},
		"SCOSC and SCORC after a wide character": {
			height: 3, output: wide + "\x1b[s\x1b[ux",
			want: shownScreen{rows: []string{a(38) + " x", "", ""}, cursorX: lastColumn},
		},
		"?1048h and ?1048l after a wide character": {
			height: 3, output: wide + "\x1b[?1048h\x1b[?1048lx",
			want: shownScreen{rows: []string{a(38) + " x", "", ""}, cursorX: lastColumn},
		},
		"?1049l, sent with another mode, after a wide character on the alternate screen": {
			height: 3, output: "\x1b[?1049h" + wide + "\x1b[?25;1049lx",
			want: shownScreen{rows: []string{"x", "", ""}, cursorX: 1},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, test.height)
			writeAll(t, e, test.output)
			checkScreen(t, e, test.want)
		})
	}
}
