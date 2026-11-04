package vt

import (
	"reflect"
	"testing"
)

func TestShownCellsKeepAColumnTheSameOnEveryRow(t *testing.T) {
	emulator := NewSnapshotEmulator(4, 2)
	_, _ = emulator.WriteString("案x\r\nab")

	want := [][]string{{"案", "", "x", " "}, {"a", "b", " ", " "}}
	if got := emulator.ShownCells(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ShownCells() = %q, want %q", got, want)
	}
}

func TestShownCellsReadTheAlternateScreenWhileItIsShown(t *testing.T) {
	emulator := NewSnapshotEmulator(2, 1)
	_, _ = emulator.WriteString("ab\x1b[?1049h\x1b[Hc")
	if !emulator.AltScreenShown() {
		t.Fatal("AltScreenShown() = false after DECSET 1049")
	}
	if got := emulator.ShownCells()[0][0]; got != "c" {
		t.Fatalf("first cell = %q, want c", got)
	}

	_, _ = emulator.WriteString("\x1b[?1049l")
	if emulator.AltScreenShown() {
		t.Fatal("AltScreenShown() = true after DECRST 1049")
	}
	if got := emulator.ShownCells()[0][0]; got != "a" {
		t.Fatalf("first cell = %q, want a", got)
	}
}
