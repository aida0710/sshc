package terminal

// maxControlSequenceBytes bounds what one control sequence may collect. The
// sequences read here are under 16 bytes; a longer one is something else.
const maxControlSequenceBytes = 32

type controlSequenceState uint8

const (
	sequenceText controlSequenceState = iota
	sequenceEscape
	sequenceCollect
	sequenceDiscard
)

// controlSequenceScanner picks the control sequences (CSI) out of the output.
// Sequences may span chunk boundaries.
type controlSequenceScanner struct {
	state   controlSequenceState
	collect []byte
	// sequence hears each sequence: its parameters and intermediates, and its
	// final byte.
	sequence func(parameters string, final byte)
	// reset hears a terminal reset (RIS); nil when it does not matter.
	reset func()
}

// Observe scans one chunk.
func (o *controlSequenceScanner) Observe(chunk []byte) {
	for _, value := range chunk {
		switch o.state {
		case sequenceText:
			if value == '\x1b' {
				o.state = sequenceEscape
			}
		case sequenceEscape:
			switch value {
			case '[':
				o.state = sequenceCollect
				o.collect = o.collect[:0]
			case 'c':
				o.state = sequenceText
				if o.reset != nil {
					o.reset()
				}
			case '\x1b':
			default:
				o.state = sequenceText
			}
		case sequenceCollect, sequenceDiscard:
			o.next(value)
		}
	}
}

// next takes one byte of a control sequence: its parameters and intermediates
// up to the final byte.
func (o *controlSequenceScanner) next(value byte) {
	switch {
	case value == '\x1b':
		o.state = sequenceEscape
	case value >= 0x40 && value <= 0x7e:
		if o.state == sequenceCollect {
			o.sequence(string(o.collect), value)
		}
		o.state = sequenceText
	case value < 0x20 || o.state == sequenceDiscard:
		// C0 controls inside a sequence act on their own and take no part in it.
	case len(o.collect) >= maxControlSequenceBytes:
		o.state = sequenceDiscard
	default:
		o.collect = append(o.collect, value)
	}
}
