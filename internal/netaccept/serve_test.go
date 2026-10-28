package netaccept

import (
	"net"
	"sync"
	"syscall"
	"testing"
	"time"
)

// failingOnceListener は、最初の Accept だけ一時的な失敗を返す。
type failingOnceListener struct {
	net.Listener
	once sync.Once
}

func (listener *failingOnceListener) Accept() (net.Conn, error) {
	var failed bool
	listener.once.Do(func() { failed = true })
	if failed {
		return nil, &net.OpError{Op: "accept", Err: syscall.EMFILE}
	}
	return listener.Listener.Accept()
}

func TestAcceptingContinuesAfterATemporaryFailure(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := &failingOnceListener{Listener: inner}
	handled := make(chan struct{}, 1)
	stopped := make(chan struct{})
	go func() {
		Serve(listener, 0, func(connection net.Conn) {
			_ = connection.Close()
			handled <- struct{}{}
		})
		close(stopped)
	}()
	connection, err := net.Dial("tcp", inner.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection after a temporary failure was not handled")
	}
	_ = inner.Close()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after the listener closed")
	}
}

func TestConnectionsBeyondTheLimitAreClosedAndFreedSlotsAreReused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	started := make(chan struct{}, 3)
	release := make(chan struct{}, 3)
	go Serve(listener, 2, func(conn net.Conn) {
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
			t.Cleanup(func() { _ = third.Close() })
			release <- struct{}{}
			release <- struct{}{}
			return
		case <-time.After(10 * time.Millisecond):
			_ = third.Close()
			if time.Now().After(reuseDeadline) {
				t.Fatal("the listener did not reuse a released connection slot")
			}
		}
	}
}
