package telnet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"sshc/internal/terminal"
)

func TestReadResumesACommandThatAReadTimeoutSplit(t *testing.T) {
	t.Parallel()
	// The stream holds an escaped IAC, a negotiation, a TERMINAL-TYPE SEND
	// subnegotiation and a trailing negotiation, so every decoder state is a
	// possible place for the read deadline to expire.
	stream := []byte{
		'a', commandIAC, commandIAC,
		commandIAC, commandDO, optionTerminalType,
		commandIAC, commandSB, optionTerminalType, terminalTypeSEND, commandIAC, commandSE,
		commandIAC, commandWILL, optionEcho,
		'z',
	}
	wantApplication := []byte{'a', commandIAC, 'z'}
	wantReplies := []byte{commandIAC, commandWILL, optionTerminalType}
	wantReplies = append(wantReplies, commandIAC, commandSB, optionTerminalType, terminalTypeIS)
	wantReplies = append(wantReplies, terminal.DefaultTerminalType...)
	wantReplies = append(wantReplies, commandIAC, commandSE, commandIAC, commandDO, optionEcho)

	for split := 1; split < len(stream); split++ {
		raw := &scriptedConn{reads: [][]byte{stream[:split], nil, stream[split:]}}
		connection := scriptedConnection(t, raw)
		if err := connection.SetReadTimeout(time.Second); err != nil {
			t.Fatal(err)
		}

		application, err := readUntilEOF(connection)
		if err != nil {
			t.Fatalf("split after %d bytes: Read error = %v, want the rest of the stream", split, err)
		}
		if !bytes.Equal(application, wantApplication) {
			t.Fatalf("split after %d bytes: application data = %v, want %v", split, application, wantApplication)
		}
		if replies := raw.writtenBytes(); !bytes.Equal(replies, wantReplies) {
			t.Fatalf("split after %d bytes: replies = %v, want %v", split, replies, wantReplies)
		}
	}
}

func TestReadTimeoutInsideASubnegotiationKeepsTheConnectionOpen(t *testing.T) {
	t.Parallel()
	raw := &scriptedConn{reads: [][]byte{
		{commandIAC, commandSB, optionTerminalType},
		nil,
	}}
	connection := scriptedConnection(t, raw)
	if err := connection.SetReadTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	count, err := connection.Read(make([]byte, 8))
	if count != 0 || err != nil {
		t.Fatalf("Read = %d, %v; want the expired deadline reported as no data", count, err)
	}
	if raw.isClosed() {
		t.Fatal("an expired read deadline closed the connection")
	}
}

func readUntilEOF(connection *Conn) ([]byte, error) {
	var application []byte
	buffer := make([]byte, 64)
	for {
		count, err := connection.Read(buffer)
		application = append(application, buffer[:count]...)
		if errors.Is(err, io.EOF) {
			return application, nil
		}
		if err != nil {
			return application, err
		}
	}
}

func scriptedConnection(t *testing.T, raw *scriptedConn) *Conn {
	t.Helper()
	connection, err := (Dialer{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return raw, nil
	}}).Dial(context.Background(), Config{Address: "router"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

// scriptedConn returns its reads in the listed order so a read deadline can
// expire at an exact byte boundary without depending on scheduling. A nil
// entry is one expired deadline; after the last entry the peer closes.
type scriptedConn struct {
	net.Conn

	mu      sync.Mutex
	reads   [][]byte
	written bytes.Buffer
	closed  bool
}

func (connection *scriptedConn) Read(buffer []byte) (int, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.closed {
		return 0, net.ErrClosed
	}
	if len(connection.reads) == 0 {
		return 0, io.EOF
	}
	next := connection.reads[0]
	if next == nil {
		connection.reads = connection.reads[1:]
		return 0, os.ErrDeadlineExceeded
	}
	count := copy(buffer, next)
	if count < len(next) {
		connection.reads[0] = next[count:]
	} else {
		connection.reads = connection.reads[1:]
	}
	return count, nil
}

func (connection *scriptedConn) Write(buffer []byte) (int, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.closed {
		return 0, net.ErrClosed
	}
	return connection.written.Write(buffer)
}

func (connection *scriptedConn) Close() error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.closed = true
	return nil
}

func (connection *scriptedConn) SetReadDeadline(time.Time) error { return nil }

func (connection *scriptedConn) writtenBytes() []byte {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return bytes.Clone(connection.written.Bytes())
}

func (connection *scriptedConn) isClosed() bool {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.closed
}
