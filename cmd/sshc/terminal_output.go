package main

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"golang.org/x/text/width"
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

// terminalColumns returns how many columns value takes on a terminal. East
// Asian wide and fullwidth characters, such as Japanese profile names, take two.
func terminalColumns(value string) int {
	columns := 0
	for _, character := range value {
		switch width.LookupRune(character).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			columns += 2
		default:
			columns++
		}
	}
	return columns
}

// writeAlignedRows writes label and value pairs as a human-readable table,
// padding the labels to one column width.
func writeAlignedRows(out io.Writer, rows [][2]string) {
	width := 0
	for _, row := range rows {
		width = max(width, terminalColumns(safeTerminalCell(row[0])))
	}
	for _, row := range rows {
		label := safeTerminalCell(row[0])
		fmt.Fprintf(out, "%s%s  %s\n", label, strings.Repeat(" ", width-terminalColumns(label)), safeTerminalCell(row[1]))
	}
}
