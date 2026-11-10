package terminal

// maxControlSequenceBytes は、制御シーケンス1つが集めてよい量の上限である。ここで読むシーケンスは16バイトに
// 満たない。それより長いものは別のものである。
const maxControlSequenceBytes = 32

type controlSequenceState uint8

const (
	sequenceText controlSequenceState = iota
	sequenceEscape
	sequenceCollect
	sequenceDiscard
)

// controlSequenceScanner は、出力から制御シーケンス（CSI）を拾う。シーケンスは、かたまりの境をまたぐことがある。
type controlSequenceScanner struct {
	state   controlSequenceState
	collect []byte
	// sequence は、シーケンスを1つずつ受け取る。パラメーターと中間バイト、それに終端バイトを受け取る。
	sequence func(parameters string, final byte)
	// reset は、ターミナルのリセット（RIS）を受け取る。関係が無ければ nil。
	reset func()
}

// Observe は、かたまり1つを調べる。
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

// next は、制御シーケンスのバイトを1つ受け取る。終端バイトまでの、パラメーターと中間バイトである。
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
		// シーケンスの中の C0 の制御は、それだけで働き、シーケンスには加わらない。
	case len(o.collect) >= maxControlSequenceBytes:
		o.state = sequenceDiscard
	default:
		o.collect = append(o.collect, value)
	}
}
