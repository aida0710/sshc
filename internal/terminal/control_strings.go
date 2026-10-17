package terminal

// longControlString is the length from which a control string is remembered.
// A shorter one cut in half prints at most this many stray characters, while
// remembering every OSC 7 or OSC 133 a prompt sends would cost far more memory
// than the scrollback itself. At this length a full 4 MiB scrollback holds at
// most 16384 remembered strings.
const longControlString = 256

type controlStringState uint8

const (
	controlStringGround controlStringState = iota
	controlStringEscape
	controlStringBody
	controlStringBodyEscape
)

// controlStringSpan is the absolute output range of one control string, from
// its ESC up to and excluding the first byte after its terminator.
type controlStringSpan struct {
	start uint64
	end   uint64
}

// controlStringIndex remembers where long control strings (OSC, DCS, APC, PM
// and SOS) lie in a session's output. A replay that has to begin at the oldest
// retained byte uses it to skip the rest of a string the scrollback cut in
// half: an inline image is one such string of hundreds of kilobytes, and a
// terminal that starts reading in its middle prints the base64 as text.
//
// The zero value is ready to use and starts at output position 0.
type controlStringIndex struct {
	state     controlStringState
	position  uint64
	openStart uint64
	// openIsOSC tells a BEL terminator, which ends only OSC, from data.
	openIsOSC bool
	spans     []controlStringSpan
}

func (c *controlStringIndex) Write(chunk []byte) {
	for _, value := range chunk {
		c.observe(value)
		c.position++
	}
}

func (c *controlStringIndex) observe(value byte) {
	switch c.state {
	case controlStringGround:
		if value == 0x1b {
			c.state = controlStringEscape
		}
	case controlStringEscape:
		switch value {
		case ']', 'P', '_', '^', 'X':
			c.state = controlStringBody
			c.openStart = c.position - 1
			c.openIsOSC = value == ']'
		case 0x1b:
		default:
			c.state = controlStringGround
		}
	case controlStringBody:
		switch {
		case value == 0x1b:
			c.state = controlStringBodyEscape
		case value == 0x07 && c.openIsOSC:
			c.finish(c.position + 1)
		case value == 0x18 || value == 0x1a:
			// CAN and SUB abort the string.
			c.finish(c.position + 1)
		}
	case controlStringBodyEscape:
		if value == '\\' {
			c.finish(c.position + 1)
			return
		}
		// Any other ESC ends the string and begins a new sequence with this byte.
		c.finish(c.position - 1)
		c.state = controlStringEscape
		c.observe(value)
	}
}

func (c *controlStringIndex) finish(end uint64) {
	c.state = controlStringGround
	if end-c.openStart >= longControlString {
		c.spans = append(c.spans, controlStringSpan{start: c.openStart, end: end})
	}
}

// Forget drops strings that ended before oldest, the first byte the scrollback
// still holds.
func (c *controlStringIndex) Forget(oldest uint64) {
	kept := 0
	for kept < len(c.spans) && c.spans[kept].end <= oldest {
		kept++
	}
	if kept > 0 {
		c.spans = append(c.spans[:0], c.spans[kept:]...)
	}
}

// SafeStart returns the first position at or after oldest that is not inside
// a remembered control string. A long string still open when it began before
// oldest has no end yet, so everything retained is skipped.
func (c *controlStringIndex) SafeStart(oldest uint64) uint64 {
	if c.state >= controlStringBody && c.openStart < oldest && c.position-c.openStart >= longControlString {
		return c.position
	}
	for _, span := range c.spans {
		if span.start < oldest && oldest < span.end {
			return span.end
		}
		if span.start >= oldest {
			break
		}
	}
	return oldest
}
