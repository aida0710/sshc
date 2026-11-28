# sshc local copy

This directory is `github.com/charmbracelet/x/vt` v0.1.0 and keeps its MIT
license. sshc carries it as a local package because the engine has to write out
the whole state of a terminal (both screens, the cursors saved on each, the
scroll region, the modes, the character sets and the pen), and the upstream
package does not expose most of it.

The copy holds only what sshc reaches: `safe_emulator.go`, `damage.go` and
`buffer.go` are left out. The other upstream files are unchanged. sshc adds the
`sshc_*.go` files. `sshc_snapshot.go` has `NewSnapshotEmulator` and `Snapshot`,
and the handlers the engine needs that upstream lacks (DECSTR, the kitty
keyboard flags) or that upstream makes slow (a line feed that moves rows
instead of cells), and it keeps the cursors and the margins on the screen where
upstream lets output move them off and panics. `sshc_snapshot_test.go` checks
that the handlers that are only faster draw what upstream draws.

`sshc_print.go` prints characters, ASCII without a new string for each
character, and there sshc draws what upstream does not: upstream blanks a wide
character that does not fit in the last column, and after one that fills the
last two columns it does not wait to wrap, so the next character erases it.
sshc prints as xterm.js does: the character goes to the next row and leaves the
last column empty in the pen (a cell with no content, which the session log
does not take for a space: `TextLine.WrapGap`), or with wraparound off it is
left out. Every parser handler first puts out the character held back, so
upstream's handleGrapheme is never reached.

`sshc_wide_edge.go` takes ICH and DCH (CSI Ps @ and P): it moves wide
characters whole, where upstream's ICH blanks the copy it has just moved and
upstream's DCH blanks each wide character it moves, and like xterm.js it erases
what is left of a wide character only half moved or deleted. It also erases a
wide character that a narrower `Resize` leaves cut at the right edge: the
snapshot would write it in the last column, and the browser's terminal would
wrap it to the next row and, on the bottom row, scroll the screen. The
emulator for the session log keeps it, as text the program wrote. On narrow
characters ICH and DCH draw what upstream draws (`sshc_snapshot_test.go`).

`sshc_pending_wrap.go` ends the wait to wrap after the last column on IL, DL,
RI, DECRC, SCORC (CSI u, which upstream does not take: the cursor is not
restored), `?1048l` and `?1049l`, and `shiftCells` ends it on ICH and DCH, as
xterm.js does. Upstream keeps waiting, so the next character goes to the next
row, and after IL or RI it overwrites a character there. sshc ends the wait
after a narrow character in the last column too, where it differs from
upstream.

Cases at the right edge that still differ from xterm.js include these:

- The width of a character comes from `ansi.GraphemeWidth` whether mode 2027
  is on or not, so an emoji with VS16 (❤️) takes two columns where xterm.js
  gives it one while 2027 is off; in the last column sshc moves it to the next
  row and xterm.js does not.
- With wraparound off, xterm.js erases a wide character in the last two columns
  when a wide character that does not fit is printed on its right half at the
  start of a run of printed characters, and keeps it when that happens within a
  run. A run ends at an escape sequence and at the end of each piece of output
  the browser is given, so sshc does not follow this and always keeps the
  character.
- ICH on the right half of a wide character leaves that half, moved right, as a
  cell of width 0 in xterm.js, which a snapshot cannot write. sshc leaves a
  blank in the character's background there, which the WebGL renderer draws
  alike.

`sshc_shown_screen.go` reads the screen shown as text.
`sshc_leaving_lines.go` has `NewLeavingLinesEmulator`, which keeps the lines
leaving the main screen (scrolled off its top, or erased) for the session log,
and reads output in pieces so that the scrollback, which drops its oldest line
when full, never fills. `sshc_repeat.go` takes REP (CSI Ps b), which upstream
prints as many times as asked, with no bound from the screen's size: it prints
no more than fills the screen and leaves the same screen, cursor and session
log. `sshc_bounded_counts.go` passes CHT, CBT and ECH (CSI Ps I, Z and X) to
upstream with their count held to the screen's width: upstream loops over the
count, and the parser bounds a count only to 31 bits.

Keep the other files equal to upstream v0.1.0 when updating them, and check
these files against the new internals. Check too that no upstream handler newly
loops over a count taken from the output (as REP does) without a bound from the
screen's size: the engine reads output while holding the session's lock.
`TestACountFarPastTheScreenIsReadAtOnce` tries every CSI final with the largest
count the parser passes.
