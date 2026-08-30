package sshclient

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestForwardListenerRejectsConnectionsBeyondItsLimitAndReusesSlots(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	started := make(chan struct{}, 3)
	release := make(chan struct{}, 3)
	go acceptLimited(listener, 2, func(conn net.Conn) {
		defer func() { _ = conn.Close() }()
		started <- struct{}{}
		<-release
	})

	dial := func() net.Conn {
		t.Helper()
		conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	first, second := dial(), dial()
	t.Cleanup(func() { _ = first.Close() })
	t.Cleanup(func() { _ = second.Close() })
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("a connection within the limit was not served")
		}
	}

	rejected := dial()
	defer func() { _ = rejected.Close() }()
	_ = rejected.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := rejected.Read(make([]byte, 1)); err == nil {
		t.Fatal("a connection beyond the listener limit remained open")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("a connection beyond the listener limit was not closed promptly")
	}

	release <- struct{}{}
	reuseDeadline := time.Now().Add(time.Second)
	for {
		third := dial()
		select {
		case <-started:
			defer func() { _ = third.Close() }()
			goto reused
		case <-time.After(10 * time.Millisecond):
			_ = third.Close()
			if time.Now().After(reuseDeadline) {
				t.Fatal("the listener did not reuse a released connection slot")
			}
		}
	}

reused:
	release <- struct{}{}
	release <- struct{}{}
}

type deadlineRecordingConn struct {
	net.Conn
	mutex     sync.Mutex
	deadlines []time.Time
	shorten   bool
}

func (c *deadlineRecordingConn) SetReadDeadline(deadline time.Time) error {
	c.mutex.Lock()
	c.deadlines = append(c.deadlines, deadline)
	c.mutex.Unlock()
	if c.shorten && !deadline.IsZero() {
		deadline = time.Now().Add(20 * time.Millisecond)
	}
	return c.Conn.SetReadDeadline(deadline)
}

func (c *deadlineRecordingConn) seenDeadlines() []time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return append([]time.Time(nil), c.deadlines...)
}

func TestSOCKS5NegotiationHasAReadDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()
	recorded := &deadlineRecordingConn{Conn: server, shorten: true}

	done := make(chan error, 1)
	go func() {
		_, err := readSOCKS5(recorded)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an idle SOCKS5 negotiation succeeded")
		}
		if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
			t.Fatalf("idle negotiation error = %v, want a timeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("an idle SOCKS5 negotiation had no read deadline")
	}
	deadlines := recorded.seenDeadlines()
	if len(deadlines) != 1 || deadlines[0].IsZero() {
		t.Fatalf("read deadlines = %v, want one finite negotiation deadline", deadlines)
	}
}

func TestSOCKS5NegotiationClearsItsReadDeadlineAfterSuccess(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()
	recorded := &deadlineRecordingConn{Conn: server}

	destination := make(chan string, 1)
	errors := make(chan error, 1)
	go func() {
		got, err := readSOCKS5(recorded)
		if err != nil {
			errors <- err
			return
		}
		destination <- got
	}()

	if _, err := client.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, make([]byte, 2)); err != nil {
		t.Fatal(err)
	}
	request := []byte{0x05, 0x01, 0x00, 0x01, 192, 0, 2, 1, 0, 0}
	binary.BigEndian.PutUint16(request[len(request)-2:], 443)
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(client, make([]byte, 10)); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errors:
		t.Fatal(err)
	case got := <-destination:
		if got != "192.0.2.1:443" {
			t.Fatalf("destination = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS5 negotiation did not finish")
	}
	deadlines := recorded.seenDeadlines()
	if len(deadlines) != 2 || deadlines[0].IsZero() || !deadlines[1].IsZero() {
		t.Fatalf("read deadlines = %v, want finite then cleared", deadlines)
	}
}
