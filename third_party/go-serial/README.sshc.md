# sshc local fork

This directory is based on `go.bug.st/serial` v1.8.0 and retains its BSD
3-Clause license. sshc carries it as a local package because the upstream
`Mode` API does not expose flow control.

The local changes add mutually exclusive `none`, RTS/CTS, and XON/XOFF modes
to the Unix termios and Windows DCB implementations. Port enumeration remains
on the upstream package used by `internal/serialtransport`; this fork contains
only the port operations sshc needs. Keep those implementations aligned with
upstream v1.8.0 when updating it.
