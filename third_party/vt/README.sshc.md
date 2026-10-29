# sshc local copy

This directory is `github.com/charmbracelet/x/vt` v0.1.0 and keeps its MIT
license. sshc carries it as a local package because the engine has to write out
the whole state of a terminal (both screens, the cursors saved on each, the
scroll region, the modes, the character sets and the pen), and the upstream
package does not expose most of it.

The copy holds only what sshc reaches: `safe_emulator.go`, `damage.go` and
`buffer.go` are left out. The other upstream files are unchanged. sshc adds one
file, `sshc_snapshot.go`, with `NewSnapshotEmulator` and `Snapshot`, and with
the handlers the engine needs that upstream lacks (DECSTR, the kitty keyboard
flags) or that upstream makes slow (a line feed that moves rows instead of
cells, ASCII printed without a new string for each character).
`sshc_snapshot_test.go` checks that the latter draw what upstream draws. Keep
the other files equal to upstream v0.1.0 when updating them, and check that file
against the new internals.
