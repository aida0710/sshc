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

// 1回の読み取りで、そのバイト数より多くの行が流れても（SU で何度も画面を流す）、ログの行は落ちない。
func TestALongReadKeepsEveryLineThatLeft(t *testing.T) {
	const height, scrolls = 24, 3000
	emulator := NewLeavingLinesEmulator(20, height)
	var numbered []string
	for row := 1; row <= height; row++ {
		numbered = append(numbered, fmt.Sprint(row))
	}
	writeAll(t, emulator, strings.Join(numbered, "\r\n")+strings.Repeat("\x1b[99S", scrolls))
	left := expandLeftLines(emulator.TakeLeftLines())
	if len(left) != height*scrolls || !slices.Equal(left[:height], numbered) {
		t.Fatalf("left %d lines starting %q, want %d starting with the numbered rows", len(left), left[:min(len(left), 3)], height*scrolls)
	}
}
