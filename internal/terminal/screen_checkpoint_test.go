package terminal

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"sshc/third_party/vt"
)

// recordingEmulator keeps what the checkpoint gives it, so a test can compare that with the output.
type recordingEmulator struct {
	read []byte
	// resizedAt is how much output it had read at each resize.
	resizedAt []int
	sizes     []Size
}

func (r *recordingEmulator) Write(output []byte) (int, error) {
	r.read = append(r.read, output...)
	return len(output), nil
}

func (r *recordingEmulator) Resize(cols, rows int) {
	r.resizedAt = append(r.resizedAt, len(r.read))
	r.sizes = append(r.sizes, Size{Cols: uint16(cols), Rows: uint16(rows)})
}

// AtGround reads an OSC that has not ended (ESC ] without a BEL after it) as being inside a sequence.
func (r *recordingEmulator) AtGround() bool {
	return bytes.LastIndex(r.read, []byte("\x1b]")) <= bytes.LastIndexByte(r.read, '\a')
}

func (r *recordingEmulator) Snapshot() string { return "" }

// publishTo writes chunk the way Session.publish does.
func publishTo(checkpoint *screenCheckpoint, buffer *Ring, chunk []byte) {
	checkpoint.keepUp(buffer, chunk)
	_, _ = buffer.Write(chunk)
	checkpoint.settle(buffer)
}

func TestTheCheckpointAndTheBufferTogetherHoldAllTheOutput(t *testing.T) {
	recorder := &recordingEmulator{}
	checkpoint := screenCheckpoint{newEmulator: func(Size) screenEmulator { return recorder }}
	buffer := NewRing(64)
	random := rand.New(rand.NewPCG(1, 2))
	var output []byte
	for step := range 400 {
		// Mostly small pieces, now and then one larger than the whole buffer.
		size := random.IntN(40)
		if step%37 == 0 {
			size = 64 + random.IntN(100)
		}
		chunk := make([]byte, size)
		for index := range chunk {
			chunk[index] = byte('a' + (step+index)%26)
		}
		publishTo(&checkpoint, buffer, chunk)
		output = append(output, chunk...)

		if checkpoint.position < buffer.Oldest() || checkpoint.position > buffer.Written() {
			t.Fatalf("step %d: checkpoint at %d, buffer holds %d..%d", step, checkpoint.position, buffer.Oldest(), buffer.Written())
		}
		if !bytes.Equal(recorder.read, output[:checkpoint.position]) {
			t.Fatalf("step %d: the emulator read something other than the output before %d", step, checkpoint.position)
		}
		retained, _ := buffer.ReadAvailableFrom(checkpoint.position, buffer.Capacity())
		if !bytes.Equal(retained.Data, output[checkpoint.position:]) {
			t.Fatalf("step %d: the buffer does not hold the output after the checkpoint", step)
		}
	}
}

func TestTheCheckpointStopsOutsideAnEscapeSequence(t *testing.T) {
	recorder := &recordingEmulator{}
	checkpoint := screenCheckpoint{newEmulator: func(Size) screenEmulator { return recorder }}
	buffer := NewRing(32)
	publishTo(&checkpoint, buffer, []byte("\x1b]1337;File="+strings.Repeat("A", 40)+"\aafter"))

	if !recorder.AtGround() {
		t.Fatalf("the checkpoint stopped inside the image: %q", recorder.read)
	}
	retained, _ := buffer.ReadAvailableFrom(checkpoint.position, buffer.Capacity())
	if string(retained.Data) != "after" {
		t.Fatalf("a replay would start with %q", retained.Data)
	}
}

func TestTheCheckpointTakesEachSizeWhereTheOutputReachedIt(t *testing.T) {
	recorder := &recordingEmulator{}
	checkpoint := screenCheckpoint{
		sizes:       []sizeChange{{at: 0, size: Size{Cols: 80, Rows: 24}}},
		newEmulator: func(Size) screenEmulator { return recorder },
	}
	buffer := NewRing(16)
	publishTo(&checkpoint, buffer, []byte("0123456789"))
	checkpoint.resized(buffer.Written(), Size{Cols: 100, Rows: 30})
	publishTo(&checkpoint, buffer, []byte("abcdefghij"))
	checkpoint.resized(buffer.Written(), Size{Cols: 120, Rows: 40})
	publishTo(&checkpoint, buffer, []byte(strings.Repeat("z", 40)))

	if fmt.Sprint(recorder.resizedAt) != "[10 20]" {
		t.Fatalf("resized after %v bytes, want after 10 and 20", recorder.resizedAt)
	}
	if fmt.Sprint(recorder.sizes) != fmt.Sprint([]Size{{Cols: 100, Rows: 30}, {Cols: 120, Rows: 40}}) {
		t.Fatalf("sizes = %v", recorder.sizes)
	}
}

// A browser that attaches replays the output at the size it was written for: the size in effect where the
// replay starts, then each later change at the position it was made.
func TestAReplayCarriesTheSizesItsOutputWasWrittenFor(t *testing.T) {
	session := &Session{buffer: NewRing(MinScrollback), screen: newScreenCheckpoint(Size{Cols: 80, Rows: 24}), streams: map[*Stream]bool{}}
	session.publish([]byte("written at 80 columns\r\n"))
	session.screen.resized(session.buffer.Written(), Size{Cols: 240, Rows: 60})
	wide := session.buffer.Written()
	session.publish([]byte("written at 240 columns\r\n"))
	session.screen.resized(session.buffer.Written(), Size{Cols: 120, Rows: 40})
	narrow := session.buffer.Written()
	session.publish([]byte("written at 120 columns\r\n"))

	whole, stream, ok := session.AttachFrom(0)
	if !ok {
		t.Fatal("AttachFrom(0) failed")
	}
	session.Detach(stream)
	want := []SizeAt{{At: 0, Size: Size{Cols: 80, Rows: 24}}, {At: wide, Size: Size{Cols: 240, Rows: 60}}, {At: narrow, Size: Size{Cols: 120, Rows: 40}}}
	if fmt.Sprint(whole.Sizes) != fmt.Sprint(want) {
		t.Fatalf("sizes from the start = %v, want %v", whole.Sizes, want)
	}

	later, stream, ok := session.AttachFrom(wide + 3)
	if !ok {
		t.Fatal("AttachFrom(later) failed")
	}
	session.Detach(stream)
	want = []SizeAt{{At: wide + 3, Size: Size{Cols: 240, Rows: 60}}, {At: narrow, Size: Size{Cols: 120, Rows: 40}}}
	if fmt.Sprint(later.Sizes) != fmt.Sprint(want) {
		t.Fatalf("sizes from the middle = %v, want %v", later.Sizes, want)
	}
}

// A full-screen program draws its screen once and then rewrites only what changes. Once the buffer has let the
// drawing go, a terminal that replays from the start gets the screen from the checkpoint and the rest as written.
func TestAReplayFromTheStartShowsWhatTheBufferLetGo(t *testing.T) {
	size := Size{Cols: 40, Rows: 8}
	session := &Session{buffer: NewRing(MinScrollback), screen: newScreenCheckpoint(size), streams: map[*Stream]bool{}}
	var output strings.Builder
	output.WriteString("\x1b[?1049h\x1b[?1000h\x1b[?1006h\x1b[2J")
	for row := 1; row < int(size.Rows); row++ {
		fmt.Fprintf(&output, "\x1b[%d;1H\x1b[1;3%dm● Message %d\x1b[0m", row, row%7+1, row)
	}
	for step := 0; output.Len() < 3*MinScrollback; step++ {
		fmt.Fprintf(&output, "\x1b[?2026h\x1b[%d;1H\x1b[2K%c Working %d\x1b[?2026l", size.Rows, "|/-\\"[step%4], step)
	}
	for chunk := range piecesOf(output.String(), 1000) {
		session.publish([]byte(chunk))
	}

	replay, stream, ok := session.AttachFrom(0)
	if !ok || !replay.Truncated || replay.Screen == "" {
		t.Fatalf("AttachFrom(0) = truncated %v, screen %d bytes, %v", replay.Truncated, len(replay.Screen), ok)
	}
	session.Detach(stream)

	whole := vt.NewSnapshotEmulator(int(size.Cols), int(size.Rows))
	_, _ = whole.WriteString(output.String())
	rebuilt := vt.NewSnapshotEmulator(int(size.Cols), int(size.Rows))
	_, _ = rebuilt.WriteString(replay.Screen)
	_, _ = rebuilt.Write(replay.Data)
	if rebuilt.Render() != whole.Render() || !rebuilt.IsAltScreen() {
		t.Fatalf("rebuilt screen:\n%s\nwant:\n%s", rebuilt.String(), whole.String())
	}
	if !strings.Contains(rebuilt.String(), "● Message 1") {
		t.Fatalf("the first message is missing:\n%s", rebuilt.String())
	}
}

// piecesOf cuts text into pieces of at most size bytes, the way a PTY hands output over.
func piecesOf(text string, size int) func(func(string) bool) {
	return func(yield func(string) bool) {
		for len(text) > 0 {
			piece := text[:min(size, len(text))]
			text = text[len(piece):]
			if !yield(piece) {
				return
			}
		}
	}
}

// BenchmarkFollowingOutput is how fast a session takes output once its replay buffer lets output go and the
// checkpoint follows it: coloured lines that scroll, as a build log does.
func BenchmarkFollowingOutput(b *testing.B) {
	var lines strings.Builder
	for line := range 100 {
		fmt.Fprintf(&lines, "\x1b[3%dmline %d of the build log with some words\x1b[0m\r\n", line%8, line)
	}
	chunk := []byte(lines.String())
	session := &Session{buffer: NewRing(DefaultScrollback), screen: newScreenCheckpoint(Size{Cols: 120, Rows: 40}), streams: map[*Stream]bool{}}
	b.SetBytes(int64(len(chunk)))
	for b.Loop() {
		session.publish(chunk)
	}
}
