package terminal

import "sshc/third_party/vt"

// The replay buffer (Ring) keeps only the newest output. A browser terminal that starts afresh, or that was away
// longer than the buffer reaches, can replay only that much: a program that drew its screen before it, such as a
// full-screen program that redraws only what changes (Claude Code), would be shown without what it drew. Like tmux
// with a pane, the engine follows the output with a terminal emulator of its own. It reads only what the buffer
// lets go, so its screen is the one the oldest retained byte was written on: such a browser is sent that screen
// first and then the retained output as it was (Session.readOutputLocked), which keeps the newest output exactly
// as the program wrote it.
type screenCheckpoint struct {
	// emulator has read the output before position. It is made when the buffer first lets output go: until then,
	// replaying from the first byte reproduces everything.
	emulator screenEmulator
	position uint64
	// newEmulator makes the emulator; nil makes a vt.SnapshotEmulator.
	newEmulator func(Size) screenEmulator
	// sizes are the terminal's sizes, each from the output position it was set at, oldest first. The first is the
	// size the emulator has (or is made with); the emulator takes the next once it reads up to its position.
	sizes []sizeChange
}

type sizeChange struct {
	at   uint64
	size Size
}

// screenEmulator follows the output: vt.SnapshotEmulator, or what a test puts in its place.
type screenEmulator interface {
	Write(output []byte) (int, error)
	Resize(cols, rows int)
	// AtGround reports that the output read so far does not end inside an escape sequence or a UTF-8 character.
	AtGround() bool
	// Snapshot returns the sequences that show what the emulator shows.
	Snapshot() string
}

func newScreenCheckpoint(size Size) screenCheckpoint {
	return screenCheckpoint{sizes: []sizeChange{{at: 0, size: size}}}
}

// resized records that the output from position at on is written for size.
func (c *screenCheckpoint) resized(at uint64, size Size) {
	if last := len(c.sizes) - 1; last > 0 && c.sizes[last].at == at {
		c.sizes[last].size = size
	} else {
		c.sizes = append(c.sizes, sizeChange{at: at, size: size})
	}
	c.takeSizes()
}

// keepUp reads the output that writing chunk is about to push out of buffer. Call it before the write.
func (c *screenCheckpoint) keepUp(buffer *Ring, chunk []byte) {
	written := buffer.Written()
	end := written + uint64(len(chunk))
	if end <= uint64(buffer.Capacity()) {
		return
	}
	pushedOut := end - uint64(buffer.Capacity())
	if pushedOut <= c.position {
		return
	}
	if c.emulator == nil {
		c.emulator = c.makeEmulator()
		c.takeSizes()
	}
	if c.position < written {
		retained, _ := buffer.ReadAvailableFrom(c.position, int(min(pushedOut, written)-c.position))
		c.read(retained.Data)
	}
	if pushedOut > written {
		c.read(chunk[:pushedOut-written])
	}
}

// settleStep is how much retained output settle reads at a time. Most sequences end within it; an inline image
// takes several.
const settleStep = 256

// settle reads on from the buffer while the emulator is inside an escape sequence or a UTF-8 character, so a
// replay can start where the checkpoint is: from there the output parses as it did. Call it after the write.
func (c *screenCheckpoint) settle(buffer *Ring) {
	for c.emulator != nil && !c.emulator.AtGround() && c.position < buffer.Written() {
		retained, _ := buffer.ReadAvailableFrom(c.position, settleStep)
		for index := range retained.Data {
			c.read(retained.Data[index : index+1])
			if c.emulator.AtGround() {
				return
			}
		}
	}
}

// snapshot returns the sequences that show the checkpoint's screen and the output position they stand for. There
// is a checkpoint once the buffer has let output go.
func (c *screenCheckpoint) snapshot() (string, uint64) {
	return c.emulator.Snapshot(), c.position
}

func (c *screenCheckpoint) makeEmulator() screenEmulator {
	size := DefaultSize()
	if len(c.sizes) > 0 {
		size = c.sizes[0].size
	}
	if c.newEmulator != nil {
		return c.newEmulator(size)
	}
	return vt.NewSnapshotEmulator(int(size.Cols), int(size.Rows))
}

// read gives the emulator output from c.position on, taking each size at the position it was set at.
func (c *screenCheckpoint) read(output []byte) {
	for len(output) > 0 {
		piece := output
		if len(c.sizes) > 1 && c.sizes[1].at < c.position+uint64(len(piece)) {
			piece = output[:c.sizes[1].at-c.position]
		}
		_, _ = c.emulator.Write(piece)
		c.position += uint64(len(piece))
		output = output[len(piece):]
		c.takeSizes()
	}
}

func (c *screenCheckpoint) takeSizes() {
	if c.emulator == nil {
		return
	}
	resized := false
	for len(c.sizes) > 1 && c.sizes[1].at <= c.position {
		c.sizes = c.sizes[1:]
		resized = true
	}
	if resized {
		c.emulator.Resize(int(c.sizes[0].size.Cols), int(c.sizes[0].size.Rows))
	}
}
