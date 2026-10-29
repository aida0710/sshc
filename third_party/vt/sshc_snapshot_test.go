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
