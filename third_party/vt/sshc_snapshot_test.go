package vt

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// The snapshot emulator prints ASCII, feeds lines and scrolls its own way (sshc_snapshot.go). Given the same
// output, it must leave the screen exactly as the upstream emulator does.
func TestTheSnapshotEmulatorDrawsWhatTheUpstreamOneDraws(t *testing.T) {
	pieces := []func(*rand.Rand) string{
		func(r *rand.Rand) string { return strings.Repeat("word ", 1+r.IntN(12)) },
		func(*rand.Rand) string { return "\r\n" },
		func(*rand.Rand) string { return "\n" },
		func(*rand.Rand) string { return "\r" },
		func(*rand.Rand) string { return "\t" },
		func(*rand.Rand) string { return "\b" },
		func(r *rand.Rand) string { return fmt.Sprintf("\x1b[3%dm", r.IntN(8)) },
		func(r *rand.Rand) string { return fmt.Sprintf("\x1b[4%dm", r.IntN(8)) },
		func(*rand.Rand) string { return "\x1b[0m" },
		func(*rand.Rand) string { return "\x1b[1;4m" },
		func(r *rand.Rand) string { return fmt.Sprintf("\x1b[%d;%dH", 1+r.IntN(12), 1+r.IntN(30)) },
		func(*rand.Rand) string { return "\x1b[K" },
		func(*rand.Rand) string { return "\x1b[2J" },
		func(r *rand.Rand) string { return fmt.Sprintf("\x1b[%d;%dr", 1+r.IntN(5), 6+r.IntN(6)) },
		func(*rand.Rand) string { return "\x1b[r" },
		func(*rand.Rand) string { return "\x1b[?7l" },
		func(*rand.Rand) string { return "\x1b[?7h" },
		func(*rand.Rand) string { return "\x1b(0lqqk\x1b(B" },
		func(*rand.Rand) string { return "日本語" },
		func(*rand.Rand) string { return "é" },
		func(*rand.Rand) string { return "🎉!" },
		func(*rand.Rand) string { return "\x1bD" },
		func(*rand.Rand) string { return "\x1bM" },
		func(*rand.Rand) string { return "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\" },
		func(*rand.Rand) string { return "\x1b[?1049h" },
		func(*rand.Rand) string { return "\x1b[?1049l" },
		func(*rand.Rand) string { return "\x1b[20h" },
		func(*rand.Rand) string { return "\x1b[20l" },
	}
	random := rand.New(rand.NewPCG(7, 11))
	for round := range 300 {
		var output strings.Builder
		for range 40 + random.IntN(80) {
			output.WriteString(pieces[random.IntN(len(pieces))](random))
		}
		upstream := NewEmulator(30, 12)
		snapshot := NewSnapshotEmulator(30, 12)
		_, _ = upstream.WriteString(output.String())
		_, _ = snapshot.WriteString(output.String())
		upstream.flushGrapheme()
		snapshot.flushGrapheme()
		if got, want := snapshot.Render(), upstream.Render(); got != want {
			t.Fatalf("round %d: output %q\ndraws\n%s\nwant\n%s", round, output.String(), got, want)
		}
		if got, want := snapshot.CursorPosition(), upstream.CursorPosition(); got != want || snapshot.atPhantom != upstream.atPhantom {
			t.Fatalf("round %d: output %q leaves the cursor at %v (phantom %v), want %v (phantom %v)",
				round, output.String(), got, snapshot.atPhantom, want, upstream.atPhantom)
		}
	}
}

// An inline image is a control string far longer than the emulator keeps of one: the emulator stays inside it to
// its end, and none of it shows as text.
func TestALongControlStringIsReadToItsEnd(t *testing.T) {
	emulator := NewSnapshotEmulator(40, 10)
	image := strings.Repeat("A", 100<<10)
	_, _ = emulator.WriteString("\x1b]1337;File=inline=1:" + image[:len(image)/2])
	if emulator.AtGround() {
		t.Fatal("the emulator left the image halfway through")
	}
	_, _ = emulator.WriteString(image[len(image)/2:] + "\aafter")
	emulator.flushGrapheme()
	if !emulator.AtGround() || strings.TrimSpace(emulator.String()) != "after" {
		t.Fatalf("after the image the screen shows %q", emulator.String())
	}
}

// A program's output is not checked by anyone before the emulator reads it, and the emulator runs in the engine: a
// panic here takes every session down.
func TestOutputThatPointsOffTheScreenStaysOnIt(t *testing.T) {
	const width, height = 80, 24
	tests := map[string]struct {
		before string
		// shrink makes the terminal smaller after before, as a person does while a program writes.
		shrink bool
		after  string
	}{
		"a scroll region past the bottom, then line feeds": {
			before: "\x1b[1;30r\x1b[24;1Hab\r\n\r\ncd\r\n\r\nef",
		},
		"a scroll region past the bottom, then the cursor moved down": {
			before: "\x1b[1;30r\x1b[24;1H\x1b[5Bhello",
		},
		"a scroll region past the bottom, then inserted lines": {
			before: "\x1b[1;30r\x1b[24;1H\x1b[3Lhello",
		},
		"margins past the right, then inserted lines": {
			before: "\x1b[?69h\x1b[1;90s\x1b[1;1H\x1b[3Lhello",
		},
		"leaving the alternate screen after the terminal got smaller": {
			before: "\x1b[24;1Hhello\x1b[?1049h",
			shrink: true,
			after:  "\x1b[?1049lhello\r\nworld",
		},
		"restoring a cursor saved before the terminal got smaller": {
			before: "\x1b[24;80H\x1b7",
			shrink: true,
			after:  "\x1b8hello\r\nworld",
		},
		"a saved cursor taken back on the alternate screen after the terminal got smaller": {
			before: "\x1b[?1049h\x1b[24;80H\x1b7\x1b[?1049l",
			shrink: true,
			after:  "\x1b[?1049h\x1b8hello\r\nworld",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			emulator := NewSnapshotEmulator(width, height)
			_, _ = emulator.WriteString(test.before)
			if test.shrink {
				emulator.Resize(width/2, height/2)
			}
			_, _ = emulator.WriteString(test.after)
			if position := emulator.CursorPosition(); position.Y >= emulator.Height() || position.X >= emulator.Width() {
				t.Fatalf("cursor at %v on a screen of %dx%d", position, emulator.Width(), emulator.Height())
			}
			_ = emulator.Snapshot()
		})
	}
}

func TestAScrollRegionPastTheBottomScrollsTheWholeScreen(t *testing.T) {
	emulator := NewSnapshotEmulator(4, 2)
	_, _ = emulator.WriteString("\x1b[1;30rab\r\ncd\r\nef")
	if got := emulator.Render(); !strings.Contains(got, "cd") || !strings.Contains(got, "ef") || strings.Contains(got, "ab") {
		t.Fatalf("screen is %q, want cd and ef", got)
	}
}

// randomTerminalOutput is text and sequences chosen to move the cursor and the margins around, with numbers larger
// than the screen.
func randomTerminalOutput(random *rand.Rand) string {
	const finals = "ABCDEFGHJKLMPSTXZ@`abdefhlmnrstu"
	modes := []int{6, 7, 25, 47, 69, 1047, 1048, 1049}
	escapes := []string{"\x1b7", "\x1b8", "\x1bD", "\x1bE", "\x1bM", "\x1b[!p", "\x1bc", "\x1b(0", "\x1b(B", "\t", "\b", "\x1b#8"}
	var output strings.Builder
	for range 60 {
		switch random.IntN(6) {
		case 0:
			output.WriteString("hello 案 ❯")
		case 1:
			output.WriteString("\r\n")
		case 2:
			fmt.Fprintf(&output, "\x1b[%d;%d%c", random.IntN(120), random.IntN(120), finals[random.IntN(len(finals))])
		case 3:
			fmt.Fprintf(&output, "\x1b[%d%c", random.IntN(120), finals[random.IntN(len(finals))])
		case 4:
			fmt.Fprintf(&output, "\x1b[?%d%c", modes[random.IntN(len(modes))], "hl"[random.IntN(2)])
		case 5:
			output.WriteString(escapes[random.IntN(len(escapes))])
		}
	}
	return output.String()
}

func TestRandomOutputAtRandomSizesReadsWithoutAPanic(t *testing.T) {
	const runs, writesPerRun = 300, 8
	for seed := range runs {
		random := rand.New(rand.NewPCG(uint64(seed), 0))
		emulator := NewSnapshotEmulator(1+random.IntN(100), 1+random.IntN(40))
		for range writesPerRun {
			_, _ = emulator.WriteString(randomTerminalOutput(random))
			if random.IntN(3) == 0 {
				emulator.Resize(1+random.IntN(100), 1+random.IntN(40))
			}
			_ = emulator.Snapshot()
		}
	}
}
