package vt

import (
	"strings"
	"testing"
)

// ICH（CSI Ps @）は、カーソルの上の幅2の字も、ほかの字と同じに右へずらす。bash の readline は、日本語の前に字を
// 差し込むとき、ICH で行の残りをずらしてから字を書く。
func TestInsertingCharactersMovesTheWideCharacterUnderTheCursorRight(t *testing.T) {
	for name, test := range map[string]struct {
		output string
		want   shownScreen
	}{
		"one column": {
			output: "日本ab\x1b[1;1H\x1b[@",
			want:   shownScreen{rows: []string{" 日本ab", ""}},
		},
		"three columns": {
			output: "日本ab\x1b[1;1H\x1b[3@",
			want:   shownScreen{rows: []string{"   日本ab", ""}},
		},
		"readline puts a character before Japanese": {
			output: "$ echo 日本\b\b\b\b\x1b[1@x",
			want:   shownScreen{rows: []string{"$ echo x日本", ""}, cursorX: 8},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, 2)
			writeAll(t, e, test.output)
			checkScreen(t, e, test.want)
		})
	}
}

// カーソルが幅2の字の右半分にあるときの ICH は、xterm.js（BufferLine の insertCells）と同じに、先にその字を空けて
// から、行の残りを右へずらす。後ろの字は、差し込んだ桁の数だけ右へ動く。
func TestInsertingCharactersOnTheRightHalfOfAWideCharacterErasesIt(t *testing.T) {
	e := NewSnapshotEmulator(40, 2)
	writeAll(t, e, "a日本b\x1b[1;3H\x1b[@")
	checkScreen(t, e, shownScreen{rows: []string{"a   本b", ""}, cursorX: 2})
}

// DCH（CSI Ps P）は、カーソルの右の幅2の字も、ほかの字と同じに左へずらす。bash の readline は、日本語の前の字を
// 消すとき、DCH で行の残りを詰める。
func TestDeletingCharactersMovesTheWideCharactersAfterTheCursorLeft(t *testing.T) {
	for name, test := range map[string]struct {
		output string
		want   shownScreen
	}{
		"one column": {
			output: "ab日本c\x1b[1;1H\x1b[P",
			want:   shownScreen{rows: []string{"b日本c", ""}},
		},
		"readline deletes a character before Japanese": {
			output: "$ echo x日本\x1b[1;8H\x1b[P",
			want:   shownScreen{rows: []string{"$ echo 日本", ""}, cursorX: 7},
		},
		"readline deletes two characters before Japanese": {
			output: "$ echo xy日本\x1b[1;8H\x1b[2P",
			want:   shownScreen{rows: []string{"$ echo 日本", ""}, cursorX: 7},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, 2)
			writeAll(t, e, test.output)
			checkScreen(t, e, test.want)
		})
	}
}

// DCH で幅2の字の片方の半分だけを消すと、xterm.js（BufferLine の deleteCells）と同じに、残った半分を空白にする。
// 後ろの字は、消した桁の数だけ左へ動く。
func TestDeletingHalfOfAWideCharacterErasesTheOtherHalf(t *testing.T) {
	for name, test := range map[string]struct {
		output string
		want   shownScreen
	}{
		"the left half": {
			output: "日本ab\x1b[1;1H\x1b[P",
			want:   shownScreen{rows: []string{" 本ab", ""}},
		},
		"the right half": {
			output: "a日本b\x1b[1;3H\x1b[P",
			want:   shownScreen{rows: []string{"a 本b", ""}, cursorX: 2},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := NewSnapshotEmulator(40, 2)
			writeAll(t, e, test.output)
			checkScreen(t, e, test.want)
		})
	}
}

// 画面を狭めて右半分を切られた幅2の字は、空いたマスになる。スナップショットがその字を最後の桁に書くと、
// ブラウザのターミナルは次の行へ折り返し、下の行なら画面を流す。
func TestAWideCharacterCutByANarrowerScreenIsErased(t *testing.T) {
	e := NewSnapshotEmulator(40, 2)
	writeAll(t, e, strings.Repeat("a", 38)+"日\r\n")
	e.Resize(39, 2)
	checkScreen(t, e, shownScreen{rows: []string{strings.Repeat("a", 38), ""}, cursorY: 1})
	if snapshot := e.Snapshot(); strings.Contains(snapshot, "日") {
		t.Fatalf("the snapshot still writes the cut character: %q", snapshot)
	}
}
