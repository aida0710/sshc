package vt

import (
	"slices"
	"testing"
)

func writeAll(t *testing.T, e *SnapshotEmulator, text string) {
	t.Helper()
	if _, err := e.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
}

func TestLinesScrolledOffTheTopLeaveTheScreenInOrder(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 3)
	writeAll(t, e, "one\r\ntwo\r\nthree\r\nfour\r\nfive")
	if got, want := e.TakeLeftLines(), []string{"one", "two"}; !slices.Equal(got, want) {
		t.Fatalf("left = %q, want %q", got, want)
	}
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left again = %q, want nothing new", got)
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
		t.Fatalf("left = %q, want nothing while the rows were redrawn", got)
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
	if got, want := e.TakeLeftLines(), []string{"ls", "", "file", "$ clear"}; !slices.Equal(got, want) {
		t.Fatalf("left = %q, want %q", got, want)
	}
}

func TestWhatAFullScreenProgramDrawsNeverLeavesIntoTheLog(t *testing.T) {
	e := NewLeavingLinesEmulator(20, 3)
	writeAll(t, e, "$ vim\r\n\x1b[?1049h")
	writeAll(t, e, "a\r\nb\r\nc\r\nd\r\ne\x1b[2J")
	writeAll(t, e, "\x1b[?1049l$ ")
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left = %q, want nothing from the alternate screen", got)
	}
	if got, want := e.MainScreenLines(), []string{"$ vim", "$"}; !slices.Equal(got, want) {
		t.Fatalf("screen = %q, want %q", got, want)
	}
}

func TestTheSnapshotEmulatorKeepsNoLeavingLines(t *testing.T) {
	e := NewSnapshotEmulator(20, 2)
	writeAll(t, e, "one\r\ntwo\r\nthree\r\n")
	if got := e.TakeLeftLines(); got != nil {
		t.Fatalf("left = %q, want none from an emulator that is not keeping them", got)
	}
}
