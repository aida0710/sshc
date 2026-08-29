package terminal_test

import (
	"strings"
	"testing"

	"sshc/internal/terminal"
)

func TestPlainTextRemovesTerminalControlsWithoutDamagingText(t *testing.T) {
	input := []byte("\x1b[31m赤\x1b[0m\r\nplain\ttext\x00\x7f")
	if got := terminal.PlainText(input); got != "赤\nplain\ttext" {
		t.Fatalf("PlainText() = %q", got)
	}
}

func TestPlainTextDoesNotExposeOSCOrControlStringPayloads(t *testing.T) {
	input := []byte("before\x1b]8;;https://secret.invalid\x1b\\link\x1b]8;;\x1b\\" +
		"\x1bPprivate terminal data\x1b\\after")
	got := terminal.PlainText(input)
	if got != "beforelinkafter" || strings.Contains(got, "secret") || strings.ContainsRune(got, '\x1b') {
		t.Fatalf("PlainText() = %q", got)
	}
}

func TestPlainTextTurnsStandaloneCarriageReturnsIntoTranscriptLines(t *testing.T) {
	if got := terminal.PlainText([]byte("10%\r20%\r\nDone")); got != "10%\n20%\nDone" {
		t.Fatalf("PlainText() = %q", got)
	}
}

func TestPlainTextFromCarriesEscapeStateAcrossTheCursor(t *testing.T) {
	for _, test := range []struct {
		input string
		emit  int
		want  string
	}{
		{"before\x1b[31mred\x1b[0mafter", len("before\x1b[3"), "redafter"},
		{"before\x1b]8;;https://secret.invalid\x1b\\link\x1b]8;;\x1b\\after", len("before\x1b]8;;http"), "linkafter"},
	} {
		got := terminal.PlainTextFrom([]byte(test.input), test.emit)
		if got != test.want || strings.ContainsRune(got, '\x1b') || strings.Contains(got, "31m") || strings.Contains(got, "secret") {
			t.Errorf("PlainTextFrom(%q, %d) = %q, want %q", test.input, test.emit, got, test.want)
		}
	}
}
