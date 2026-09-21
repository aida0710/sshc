package terminal

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// PlainTextFrom removes terminal control sequences and control characters from
// a bounded scrollback fragment, producing a readable transcript rather than a
// reconstruction of the screen. It decodes terminal state from the beginning of
// input but emits only characters whose source begins at or after emitFrom;
// callers pass a cursor that may land inside an escape sequence, or 0 for all.
func PlainTextFrom(input []byte, emitFrom int) string {
	if emitFrom < 0 {
		emitFrom = 0
	}
	const (
		plain = iota
		escape
		csi
		osc
		controlString
	)
	state := plain
	escapeInString := false
	pendingCarriageReturn := false
	pendingCarriageReturnAt := 0
	var output strings.Builder
	output.Grow(len(input))
	flushCarriageReturn := func(next byte) {
		if pendingCarriageReturn {
			if next != '\n' && pendingCarriageReturnAt >= emitFrom {
				output.WriteByte('\n')
			}
			pendingCarriageReturn = false
		}
	}

	for index := 0; index < len(input); {
		value := input[index]
		switch state {
		case plain:
			flushCarriageReturn(value)
			switch {
			case value == 0x1b:
				state = escape
				index++
			case value == '\r':
				pendingCarriageReturn = true
				pendingCarriageReturnAt = index
				index++
			case value == '\n' || value == '\t':
				if index >= emitFrom {
					output.WriteByte(value)
				}
				index++
			case value < 0x20 || value == 0x7f:
				index++
			case value < utf8.RuneSelf:
				if index >= emitFrom {
					output.WriteByte(value)
				}
				index++
			default:
				character, size := utf8.DecodeRune(input[index:])
				if character == utf8.RuneError && size == 1 {
					index++
					continue
				}
				if index >= emitFrom && !unicode.IsControl(character) {
					output.WriteRune(character)
				}
				index += size
			}
		case escape:
			switch value {
			case '[':
				state = csi
			case ']':
				state = osc
				escapeInString = false
			case 'P', 'X', '^', '_':
				state = controlString
				escapeInString = false
			default:
				state = plain
			}
			index++
		case csi:
			// ECMA-48 CSI ends at its first final byte.
			if value >= 0x40 && value <= 0x7e {
				state = plain
			}
			index++
		case osc:
			if value == 0x07 {
				state = plain
				escapeInString = false
			} else if escapeInString && value == '\\' {
				state = plain
				escapeInString = false
			} else {
				escapeInString = value == 0x1b
			}
			index++
		case controlString:
			if escapeInString && value == '\\' {
				state = plain
				escapeInString = false
			} else {
				escapeInString = value == 0x1b
			}
			index++
		}
	}
	flushCarriageReturn(0)
	return output.String()
}
