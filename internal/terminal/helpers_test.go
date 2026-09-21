package terminal_test

import (
	"testing"

	"sshc/internal/terminal"
)

// snapshotOf は、いまスクロールバックに残っている出力を、製品と同じ経路
// （先頭からの AttachFrom）で読む。
func snapshotOf(session *terminal.Session) []byte {
	replay, stream, ok := session.AttachFrom(0)
	if !ok {
		return nil
	}
	session.Detach(stream)
	return replay.Data
}

// attach は、バッファの内容とライブの出力への stream を返す。
func attach(t *testing.T, session *terminal.Session) ([]byte, *terminal.Stream) {
	t.Helper()
	replay, stream, ok := session.AttachFrom(0)
	if !ok {
		t.Fatal("AttachFrom(0) refused the start of the buffer")
	}
	return replay.Data, stream
}

// ringContents は、リングが今保持しているバイト列を先頭から読む。
func ringContents(ring *terminal.Ring) []byte {
	read, ok := ring.ReadAvailableFrom(0)
	if !ok {
		return nil
	}
	return read.Data
}
