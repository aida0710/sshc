package main

import (
	"strings"
	"testing"
)

func TestSafeTerminalCellBoundsHumanOutput(t *testing.T) {
	input := strings.Repeat("界", maxHumanCellRunes+1)
	got := safeTerminalCell(input)
	if !strings.HasSuffix(got, "…") || len([]rune(got)) != maxHumanCellRunes+1 {
		t.Fatalf("bounded output has %d runes and suffix %q", len([]rune(got)), got[len(got)-3:])
	}
}
