package main

import (
	"fmt"
	"strings"
	"unicode"
)

const maxHumanCellRunes = 4096

// safeTerminalCell keeps human-readable CLI tables on one logical line and
// prevents values supplied by configuration or a remote sync manifest from
// being interpreted as terminal control sequences. JSON output deliberately
// retains the original values for machine consumers.
func safeTerminalCell(value string) string {
	var output strings.Builder
	count := 0
	for _, character := range value {
		if count == maxHumanCellRunes {
			output.WriteRune('…')
			break
		}
		if unicode.IsControl(character) {
			if character <= 0xffff {
				_, _ = fmt.Fprintf(&output, "\\u%04X", character)
			} else {
				_, _ = fmt.Fprintf(&output, "\\U%08X", character)
			}
		} else {
			output.WriteRune(character)
		}
		count++
	}
	return output.String()
}
