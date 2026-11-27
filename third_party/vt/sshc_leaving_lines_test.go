package vt

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func writeAll(t *testing.T, e *SnapshotEmulator, text string) {
	t.Helper()
	if _, err := e.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
}

// expandLeftLines は、続けて同じ行が流れたことを Repeats で表した行を、1行ずつに戻す。
func expandLeftLines(lines []LeftLine) []string {
	var texts []string
	for _, line := range lines {
		for range 1 + line.Repeats {
			texts = append(texts, line.Text)
		}
	}
	return texts
}

func TestLinesScrolledOffTheTopLeaveTheScreenInOrder(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 3)
	writeAll(t, e, "one\r\ntwo\r\nthree\r\nfour\r\nfive")
	if got, want := expandLeftLines(e.TakeLeftLines()), []string{"one", "two"}; !slices.Equal(got, want) {
		t.Fatalf("left = %q, want %q", got, want)
	}
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left again = %v, want nothing new", got)
	}
	if got, want := e.MainScreenLines(), []string{"three", "four", "five"}; !slices.Equal(got, want) {
		t.Fatalf("screen = %q, want %q", got, want)
	}
}

// A program that redraws the same rows (a spinner, a status that changes) leaves only what scrolls away.
func TestRowsRedrawnInPlaceDoNotLeaveTheScreen(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 4)
	writeAll(t, e, "$ run\r\n")
	for _, frame := range []string{"⠋ working", "⠙ working", "⠹ working"} {
		writeAll(t, e, "\r\x1b[2K"+frame)
	}
	writeAll(t, e, "\r\x1b[2Kdone\r\n")
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left = %v, want nothing while the rows were redrawn", got)
	}
	if got, want := e.MainScreenLines(), []string{"$ run", "done"}; !slices.Equal(got, want) {
		t.Fatalf("screen = %q, want %q", got, want)
	}
}

func TestErasingTheScreenLeavesItsRowsWithTheBlankOnesBetween(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 5)
	writeAll(t, e, "ls\r\n\r\nfile\r\n$ clear\r\n")
	// What clear sends: home, erase the screen, erase the scrollback.
	writeAll(t, e, "\x1b[H\x1b[2J\x1b[3J$ ")
	if got, want := expandLeftLines(e.TakeLeftLines()), []string{"ls", "", "file", "$ clear"}; !slices.Equal(got, want) {
		t.Fatalf("left = %q, want %q", got, want)
	}
}

func TestWhatAFullScreenProgramDrawsNeverLeavesIntoTheLog(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 3)
	writeAll(t, e, "$ vim\r\n\x1b[?1049h")
	writeAll(t, e, "a\r\nb\r\nc\r\nd\r\ne\x1b[2J")
	writeAll(t, e, "\x1b[?1049l$ ")
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left = %v, want nothing from the alternate screen", got)
	}
	if got, want := e.MainScreenLines(), []string{"$ vim", "$"}; !slices.Equal(got, want) {
		t.Fatalf("screen = %q, want %q", got, want)
	}
}

func TestTheSnapshotEmulatorKeepsNoLeavingLines(t *testing.T) {
	e := NewSnapshotEmulator(20, 2)
	writeAll(t, e, "one\r\ntwo\r\nthree\r\n")
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left = %v, want none from an emulator that is not keeping them", got)
	}
}

// numberedRows は、1 から height までの番号を書いた行である。流れた行の先頭が落ちていないかを、番号で見る。
func numberedRows(height int) []string {
	var numbered []string
	for row := 1; row <= height; row++ {
		numbered = append(numbered, fmt.Sprint(row))
	}
	return numbered
}

// 1回の読み取りで、そのバイト数より多くの行が流れても（SU で何度も画面を流す）、ログの行は落ちない。
func TestALongReadKeepsEveryLineThatLeft(t *testing.T) {
	const height, scrolls = 24, 3000
	emulator := NewLeavingLinesEmulator(20, height)
	numbered := numberedRows(height)
	writeAll(t, emulator, strings.Join(numbered, "\r\n")+strings.Repeat("\x1b[99S", scrolls))
	left := expandLeftLines(emulator.TakeLeftLines())
	if len(left) != height*scrolls || !slices.Equal(left[:height], numbered) {
		t.Fatalf("left %d lines starting %q, want %d starting with the numbered rows", len(left), left[:min(len(left), 3)], height*scrolls)
	}
}

// ASCII のバイトが無いまま長く続く出力を1回で読んでも、ログの行は落ちない。区切りは ASCII のバイトの前まで延ばすが、
// ASCII でないバイトにも、1バイトで1行を流すもの（C1 の IND、0x84）がある。
func TestALongReadWithNoASCIIKeepsEveryLineThatLeft(t *testing.T) {
	// IND は上流の index で流れ、画面のマスを1つずつ動かすので、画面を小さくする。
	const width, height = 4, 4
	emulator := NewLeavingLinesEmulator(width, height)
	numbered := numberedRows(height)
	writeAll(t, emulator, strings.Join(numbered, "\r\n"))
	// 主画面の scrollback が持てるより多くの行を流す。
	indexes := leavingLinesCapacity + height
	writeAll(t, emulator, strings.Repeat("\x84", indexes))
	left := expandLeftLines(emulator.TakeLeftLines())
	if len(left) != indexes || !slices.Equal(left[:height], numbered) {
		t.Fatalf("left %d lines starting %q, want %d starting with the numbered rows", len(left), left[:min(len(left), 3)], indexes)
	}
}

// 流れた行を残すエミュレーターは出力を区切って読むが、区切りで字を割らない。結合文字や ZWJ でつないだ字が
// 区切りをまたいでも、1回で読んだときと同じ画面と、同じ流れた行になる。
func TestReadingInPiecesKeepsCharactersThatSpanThePieces(t *testing.T) {
	const width, height = 20, 6
	var output strings.Builder
	for line := range 600 {
		output.WriteString("e\u0301\u0302\u0303日本\U0001F468\u200d\U0001F469\u200d\U0001F467")
		if line%3 == 2 {
			output.WriteString("\r\n")
		}
	}
	pieces, whole := NewLeavingLinesEmulator(width, height), NewLeavingLinesEmulator(width, height)
	writeAll(t, pieces, output.String())
	if _, err := whole.Emulator.Write([]byte(output.String())); err != nil {
		t.Fatal(err)
	}
	if got, want := pieces.Snapshot(), whole.Snapshot(); got != want {
		t.Fatalf("read in pieces, the screen is\n%q\nwant\n%q", got, want)
	}
	if got, want := expandLeftLines(pieces.TakeLeftLines()), expandLeftLines(whole.TakeLeftLines()); !slices.Equal(got, want) {
		t.Fatalf("read in pieces, %d lines left, want %d", len(got), len(want))
	}
}

// ASCII のバイトが無いまま、区切りを延ばしきれないほど出力が続くと、字の途中で区切る。それでも、1つの字は割れず、
// 1回で読んだときと同じ画面と、同じ流れた行になる。前に置く字で区切りの位置をずらし、3バイトの字のどの位置でも
// 区切る。
func TestCuttingInsideACharacterKeepsTheCharacter(t *testing.T) {
	const width, height = 20, 6
	run := strings.Repeat("日", 2*pieceExtensionLimit/len("日"))
	for _, before := range []string{"", "é", "éé"} {
		output := before + run
		pieces, whole := NewLeavingLinesEmulator(width, height), NewLeavingLinesEmulator(width, height)
		writeAll(t, pieces, output)
		if _, err := whole.Emulator.Write([]byte(output)); err != nil {
			t.Fatal(err)
		}
		if got, want := pieces.Snapshot(), whole.Snapshot(); got != want {
			t.Fatalf("after %q, read in pieces, the screen is\n%q\nwant\n%q", before, got, want)
		}
		if got, want := expandLeftLines(pieces.TakeLeftLines()), expandLeftLines(whole.TakeLeftLines()); !slices.Equal(got, want) {
			t.Fatalf("after %q, read in pieces, %d lines left, want %d", before, len(got), len(want))
		}
	}
}
