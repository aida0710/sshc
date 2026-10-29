package terminal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/third_party/vt"
)

// screenSnapshotFixtures is where the screens written out by the engine are kept for web/src/terminal/screenSnapshot.test.ts,
// which writes each one into xterm.js with the output after it and compares that terminal with one that read the
// whole output. Run this test with SSHC_UPDATE_TESTDATA=1 to write the file again after changing the snapshot.
var screenSnapshotFixtures = filepath.Join("testdata", "screen-snapshots.json")

type screenSnapshotFixture struct {
	Name string `json:"name"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
	// Output is everything the program wrote; Tail is what it wrote after the checkpoint.
	Output string `json:"output"`
	Tail   string `json:"tail"`
	// Screen is what the engine sends for the checkpoint.
	Screen string `json:"screen"`
}

func snapshotFixture(t *testing.T, name string, cols, rows int, before, after string) screenSnapshotFixture {
	t.Helper()
	emulator := vt.NewSnapshotEmulator(cols, rows)
	_, _ = emulator.WriteString(before)
	if !emulator.AtGround() {
		t.Fatalf("%s: the checkpoint is inside an escape sequence", name)
	}
	return screenSnapshotFixture{Name: name, Cols: cols, Rows: rows, Output: before + after, Tail: after, Screen: emulator.Snapshot()}
}

func screenSnapshotCases(t *testing.T) []screenSnapshotFixture {
	var fullScreen strings.Builder
	fullScreen.WriteString("\x1b[?1049h\x1b[?1000h\x1b[?1002h\x1b[?1006h\x1b[?2004h\x1b[>1u\x1b[2J")
	for row := 1; row < 10; row++ {
		fmt.Fprintf(&fullScreen, "\x1b[%d;1H\x1b[1;38;5;%dm●\x1b[0m Message %d \x1b[48;2;30;60;90m done \x1b[0m", row, 160+row, row)
	}
	var shell strings.Builder
	for line := 1; line <= 30; line++ {
		fmt.Fprintf(&shell, "\x1b[3%dmline %d\x1b[0m of the build log\r\n", line%8, line)
	}
	shell.WriteString("$ ")

	return []screenSnapshotFixture{
		snapshotFixture(t, "a full-screen program that rewrites one row", 40, 12, fullScreen.String(),
			"\x1b[?2026h\x1b[12;1H\x1b[2K| Working 7\x1b[?2026l\x1b[5;3Hchanged"),
		snapshotFixture(t, "leaving the alternate screen gives the normal screen and its cursor back", 40, 10,
			"$ build\r\nok\r\n$ claude\x1b[?1049h\x1b[2J\x1b[Hdrawn on the alternate screen", "\x1b[?1049l\r\n$ "),
		snapshotFixture(t, "a shell whose output scrolled", 50, 8, shell.String(), "make test\r\nPASS\r\n$ "),
		snapshotFixture(t, "wide characters and emoji keep the rest of the row in place", 40, 6,
			"日本語のテキスト 🎉 rest\r\n● ⏺ ✻ marks\r\n", "\x1b[1;1H\x1b[30Cend"),
		snapshotFixture(t, "a row filled to the last column waits to wrap", 10, 4, "0123456789", "X"),
		snapshotFixture(t, "the saved cursor and the pen survive", 30, 6,
			"\x1b[3;5H\x1b[4;35m\x1b7\x1b[1;1Hhome\x1b[0m", "\x1b8saved"),
		snapshotFixture(t, "a scroll region with origin mode", 30, 10,
			"top\x1b[3;7r\x1b[?6h\x1b[2;1Hin region", "\x1b[1;1Hfirst\x1bD\x1bD\x1bD\x1bD\x1bD\x1bDafter scrolling\x1b[r"),
		snapshotFixture(t, "line drawing characters keep their character set", 20, 4, "\x1b(0lqqk\r\n", "x  x\r\nmqqj\x1b(B ok"),
		snapshotFixture(t, "modes and keys carry over", 30, 4,
			"\x1b[?1h\x1b=\x1b[?2004h\x1b[?1004h\x1b[4h\x1b[?25l\x1b[5 q", "ab\x1b[1;1HX"),
		snapshotFixture(t, "a link stays a link", 40, 4, "see \x1b]8;;https://example.com/docs\x1b\\the docs\x1b]8;;\x1b\\ here", "\r\nnext"),
		snapshotFixture(t, "wraparound off", 10, 4, "\x1b[?7l0123456789", "XYZ"),
		snapshotFixture(t, "rows scrolled with a background colour", 20, 4,
			"\x1b[44mone\r\ntwo\r\nthree\r\nfour\r\nfive\r\nsix", "\x1b[0m\r\nseven"),
		snapshotFixture(t, "a reset forgets the kitty keyboard flags", 20, 4, "\x1b[>5u\x1b[>1u\x1bcafter reset", "\r\nnext"),
		snapshotFixture(t, "a soft reset puts the modes and the pen back", 30, 4,
			"\x1b[?1h\x1b[4h\x1b[?2004h\x1b[>3u\x1b[31m\x1b[2;3r\x1b[!p", "plain\r\nlines\r\nscroll\r\nfreely"),
	}
}

func TestScreenSnapshotFixturesAreCurrent(t *testing.T) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(screenSnapshotCases(t)); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("SSHC_UPDATE_TESTDATA") == "1" {
		if err := os.WriteFile(screenSnapshotFixtures, encoded.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	stored, err := os.ReadFile(screenSnapshotFixtures)
	if err != nil || !bytes.Equal(stored, encoded.Bytes()) {
		t.Fatalf("%s is not what the engine writes now; run this test with SSHC_UPDATE_TESTDATA=1", screenSnapshotFixtures)
	}
}
