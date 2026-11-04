package vt

// sshc's addition to its copy of the package (README.sshc.md). The sshc engine reads what a terminal session shows,
// to tell when a program that cannot report it waits for the person.

// AltScreenShown reports whether the alternate screen is the one shown.
func (e *SnapshotEmulator) AltScreenShown() bool { return e.altScreenShown() }

// ShownCells returns the text of every cell of the screen shown, row by row: " " for an empty cell and "" for the
// second cell of a wide character, so that a column means the same column on every row.
func (e *SnapshotEmulator) ShownCells() [][]string {
	e.flushGrapheme()
	rows := make([][]string, e.Height())
	for y := range rows {
		rows[y] = make([]string, e.Width())
		for x := range rows[y] {
			rows[y][x] = e.CellAt(x, y).Content
		}
	}
	return rows
}
