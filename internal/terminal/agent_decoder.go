package terminal

import "encoding/base64"

var agentOSCPrefix = []byte{'\x1b', ']', '6', '9', '7', '3', ';'}

type agentDecoderState uint8

const (
	agentDecoderText agentDecoderState = iota
	agentDecoderPayload
	agentDecoderEscape
	agentDecoderDiscard
	agentDecoderDiscardEscape
)

// agentDecoder removes only sshc's private OSC channel. It keeps a bounded
// prefix/payload buffer so arbitrary terminal output cannot grow memory.
type agentDecoder struct {
	state   agentDecoderState
	prefix  []byte
	payload []byte
	onEvent func(agentEvent)
}

func newAgentDecoder(onEvent func(agentEvent)) *agentDecoder {
	return &agentDecoder{onEvent: onEvent}
}

func (d *agentDecoder) Write(chunk []byte) []byte {
	output := make([]byte, 0, len(chunk))
	for _, value := range chunk {
		switch d.state {
		case agentDecoderText:
			d.prefix = append(d.prefix, value)
			for len(d.prefix) > 0 && !isAgentPrefix(d.prefix) {
				output = append(output, d.prefix[0])
				d.prefix = d.prefix[1:]
			}
			if len(d.prefix) == len(agentOSCPrefix) {
				d.prefix = d.prefix[:0]
				d.payload = d.payload[:0]
				d.state = agentDecoderPayload
			}
		case agentDecoderPayload:
			switch value {
			case '\a':
				d.complete()
			case '\x1b':
				d.state = agentDecoderEscape
			default:
				d.appendPayload(value)
			}
		case agentDecoderEscape:
			if value == '\\' {
				d.complete()
			} else {
				d.state = agentDecoderDiscard
			}
		case agentDecoderDiscard:
			if value == '\a' {
				d.reset()
			} else if value == '\x1b' {
				d.state = agentDecoderDiscardEscape
			}
		case agentDecoderDiscardEscape:
			if value == '\\' {
				d.reset()
			} else if value != '\x1b' {
				d.state = agentDecoderDiscard
			}
		}
	}
	return output
}

func (d *agentDecoder) Flush() []byte {
	if d.state != agentDecoderText {
		d.reset()
		return nil
	}
	flushed := append([]byte(nil), d.prefix...)
	d.prefix = d.prefix[:0]
	return flushed
}

func (d *agentDecoder) appendPayload(value byte) {
	if len(d.payload) >= MaxAgentPayload {
		d.payload = d.payload[:0]
		d.state = agentDecoderDiscard
		return
	}
	d.payload = append(d.payload, value)
}

func (d *agentDecoder) complete() {
	decoded, err := base64.RawURLEncoding.DecodeString(string(d.payload))
	if err == nil {
		if event, eventErr := decodeAgentEvent(decoded); eventErr == nil && d.onEvent != nil {
			d.onEvent(event)
		}
	}
	d.reset()
}

func (d *agentDecoder) reset() {
	d.state = agentDecoderText
	d.prefix = d.prefix[:0]
	d.payload = d.payload[:0]
}

func isAgentPrefix(candidate []byte) bool {
	if len(candidate) > len(agentOSCPrefix) {
		return false
	}
	for index := range candidate {
		if candidate[index] != agentOSCPrefix[index] {
			return false
		}
	}
	return true
}
