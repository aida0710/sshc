package telnet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestDialNormalizesAddressAndAppliesDefaults(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	var network, address string
	connection, err := (Dialer{DialContext: func(_ context.Context, gotNetwork, gotAddress string) (net.Conn, error) {
		network, address = gotNetwork, gotAddress
		return client, nil
	}}).Dial(context.Background(), Config{Address: "router.example"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if network != "tcp" || address != "router.example:23" {
		t.Fatalf("dial = %q %q, want tcp router.example:23", network, address)
	}
	if connection.terminalType != DefaultTerminalType || connection.windowWidth != 80 || connection.windowHeight != 24 {
		t.Fatalf("defaults = terminal %q size %dx%d", connection.terminalType, connection.windowWidth, connection.windowHeight)
	}
}

func TestDialPreservesIPv6ZoneAddress(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { _ = server.Close() })
	var address string
	connection, err := (Dialer{DialContext: func(_ context.Context, _, gotAddress string) (net.Conn, error) {
		address = gotAddress
		return client, nil
	}}).Dial(context.Background(), Config{Address: "[fe80::1%en0]:2323"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if address != "[fe80::1%en0]:2323" {
		t.Fatalf("dial address = %q, want IPv6 zone address", address)
	}
}

func TestNormalizeAddress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"router", "router:23"},
		{"router:2323", "router:2323"},
		{"192.0.2.1", "192.0.2.1:23"},
		{"2001:db8::1", "[2001:db8::1]:23"},
		{"[2001:db8::1]", "[2001:db8::1]:23"},
		{"[2001:db8::1]:2323", "[2001:db8::1]:2323"},
		{"fe80::1%en0", "[fe80::1%en0]:23"},
		{"[fe80::1%en0]", "[fe80::1%en0]:23"},
		{"[fe80::1%en0]:2323", "[fe80::1%en0]:2323"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := normalizeAddress(test.input)
			if err != nil || got != test.want {
				t.Fatalf("normalizeAddress(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
}

func TestDialRejectsUnsafeConfigurationBeforeNetwork(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config Config
		want   error
	}{
		{"empty address", Config{}, ErrInvalidAddress},
		{"address whitespace", Config{Address: " router"}, ErrInvalidAddress},
		{"address control", Config{Address: "router\n.example"}, ErrInvalidAddress},
		{"missing port", Config{Address: "router:"}, ErrInvalidAddress},
		{"bad bracket", Config{Address: "[router]"}, ErrInvalidAddress},
		{"bracketed hostname with port", Config{Address: "[router]:23"}, ErrInvalidAddress},
		{"zero port", Config{Address: "router:0"}, ErrInvalidAddress},
		{"port too large", Config{Address: "router:65536"}, ErrInvalidAddress},
		{"named port", Config{Address: "router:telnet"}, ErrInvalidAddress},
		{"negative port", Config{Address: "router:-1"}, ErrInvalidAddress},
		{"URL scheme", Config{Address: "telnet://router:23"}, ErrInvalidAddress},
		{"userinfo", Config{Address: "user@router:23"}, ErrInvalidAddress},
		{"path", Config{Address: "router:23/console"}, ErrInvalidAddress},
		{"backslash", Config{Address: `router\console`}, ErrInvalidAddress},
		{"query", Config{Address: "router:23?mode=raw"}, ErrInvalidAddress},
		{"fragment", Config{Address: "router:23#console"}, ErrInvalidAddress},
		{"negative timeout", Config{Address: "router", DialTimeout: -time.Second}, ErrInvalidDialTimeout},
		{"terminal whitespace", Config{Address: "router", TerminalType: "bad type"}, ErrInvalidTerminalType},
		{"terminal control", Config{Address: "router", TerminalType: "xterm\n"}, ErrInvalidTerminalType},
		{"limit negative", Config{Address: "router", MaxSubnegotiationBytes: -1}, ErrInvalidSubnegotiationLimit},
		{"limit excessive", Config{Address: "router", MaxSubnegotiationBytes: HardMaxSubnegotiationBytes + 1}, ErrInvalidSubnegotiationLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			_, err := (Dialer{DialContext: func(context.Context, string, string) (net.Conn, error) {
				called = true
				return nil, errors.New("must not dial")
			}}).Dial(context.Background(), test.config)
			if !errors.Is(err, test.want) {
				t.Fatalf("Dial() error = %v, want %v", err, test.want)
			}
			if called {
				t.Fatal("invalid configuration reached network dialer")
			}
		})
	}
}

func TestDialHonorsTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	t.Run("timeout", func(t *testing.T) {
		_, err := (Dialer{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}).Dial(context.Background(), Config{Address: "router", DialTimeout: time.Millisecond})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Dial() error = %v, want deadline exceeded", err)
		}
	})

	t.Run("parent cancellation closes established stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		connection, server := pipeConnection(t, ctx, Config{Address: "router"})
		result := make(chan error, 1)
		go func() {
			_, err := connection.Read(make([]byte, 1))
			result <- err
		}()
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Read() error = %v, want context canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("context cancellation did not unblock Read")
		}
		_ = server.Close()
	})
}

func TestDialRejectsNilConnection(t *testing.T) {
	t.Parallel()
	_, err := (Dialer{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, nil
	}}).Dial(context.Background(), Config{Address: "router"})
	if !errors.Is(err, ErrNilConnection) {
		t.Fatalf("Dial() error = %v, want nil connection error", err)
	}
}

func TestReadRemovesCommandsAndUnescapesIAC(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	go func() {
		_, _ = server.Write([]byte{'h', 'i', commandIAC, commandIAC, '!'})
	}()
	got := make([]byte, 4)
	if _, err := io.ReadFull(connection, got); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'h', 'i', commandIAC, '!'}; !bytes.Equal(got, want) {
		t.Fatalf("Read = %v, want %v", got, want)
	}
}

func TestWriteEscapesIAC(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	result := make(chan struct {
		count int
		err   error
	}, 1)
	go func() {
		count, err := connection.Write([]byte{'a', commandIAC, 'b'})
		result <- struct {
			count int
			err   error
		}{count, err}
	}()
	wire := make([]byte, 4)
	if _, err := io.ReadFull(server, wire); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'a', commandIAC, commandIAC, 'b'}; !bytes.Equal(wire, want) {
		t.Fatalf("wire = %v, want %v", wire, want)
	}
	if result := <-result; result.count != 3 || result.err != nil {
		t.Fatalf("Write = %d, %v; want 3, nil", result.count, result.err)
	}
}

func TestNVTCRRulesApplyUntilBinaryNegotiation(t *testing.T) {
	t.Run("outbound newline", func(t *testing.T) {
		connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
		result := make(chan error, 1)
		go func() {
			_, err := connection.Write([]byte{'a', '\r', 'b', '\n', 'c', '\r', '\n', 'd', '\r', 0})
			result <- err
		}()
		want := []byte{'a', '\r', '\n', 'b', '\r', '\n', 'c', '\r', '\n', 'd', '\r', 0}
		got := make([]byte, len(want))
		if _, err := io.ReadFull(server, got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("wire = %v, want %v", got, want)
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("inbound CR NUL", func(t *testing.T) {
		connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
		go func() { _, _ = server.Write([]byte{'a', '\r', 0, 'b', '\r', '\n'}) }()
		got := make([]byte, 5)
		if _, err := io.ReadFull(connection, got); err != nil {
			t.Fatal(err)
		}
		if want := []byte{'a', '\r', 'b', '\r', '\n'}; !bytes.Equal(got, want) {
			t.Fatalf("application = %v, want %v", got, want)
		}
	})
}

func TestNVTCRNULPairAcrossWriteBoundaryIsPreserved(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	result := make(chan error, 1)
	go func() {
		if _, err := connection.Write([]byte{'\r'}); err != nil {
			result <- err
			return
		}
		_, err := connection.Write([]byte{0})
		result <- err
	}()
	wire := make([]byte, 2)
	if _, err := io.ReadFull(server, wire); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'\r', 0}; !bytes.Equal(wire, want) {
		t.Fatalf("wire = %v, want %v", wire, want)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestFlushDeliversTrailingCRAsNewline(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	result := make(chan error, 1)
	go func() {
		if _, err := connection.Write([]byte{'\r'}); err != nil {
			result <- err
			return
		}
		result <- connection.Flush()
	}()
	wire := make([]byte, 2)
	if _, err := io.ReadFull(server, wire); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'\r', '\n'}; !bytes.Equal(wire, want) {
		t.Fatalf("wire = %v, want %v", wire, want)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestNVTCRLFPairAcrossChunkBoundaryIsNotDuplicated(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	payload := append(bytes.Repeat([]byte{'x'}, writeChunkBytes-1), '\r', '\n', 'z')
	result := make(chan error, 1)
	go func() {
		_, err := connection.Write(payload)
		result <- err
	}()
	wire := make([]byte, len(payload))
	if _, err := io.ReadFull(server, wire); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wire, payload) {
		t.Fatalf("wire length = %d, want unchanged CR LF pair length %d", len(wire), len(payload))
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestBinaryNegotiationDisablesNVTConversion(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1)
		_, err := connection.Read(buffer)
		readDone <- err
	}()
	if _, err := server.Write([]byte{commandIAC, commandDO, optionBinary}); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 3)
	if _, err := io.ReadFull(server, response); err != nil {
		t.Fatal(err)
	}
	if want := []byte{commandIAC, commandWILL, optionBinary}; !bytes.Equal(response, want) {
		t.Fatalf("response = %v, want %v", response, want)
	}
	if _, err := server.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}

	writeDone := make(chan error, 1)
	go func() {
		_, err := connection.Write([]byte{'\r', 0, '\n'})
		writeDone <- err
	}()
	wire := make([]byte, 3)
	if _, err := io.ReadFull(server, wire); err != nil {
		t.Fatal(err)
	}
	if want := []byte{'\r', 0, '\n'}; !bytes.Equal(wire, want) {
		t.Fatalf("wire = %v, want raw binary %v", wire, want)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	remoteRead := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data := make([]byte, 3)
		_, err := io.ReadFull(connection, data)
		remoteRead <- struct {
			data []byte
			err  error
		}{data: data, err: err}
	}()
	if _, err := server.Write([]byte{commandIAC, commandWILL, optionBinary}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(server, response); err != nil {
		t.Fatal(err)
	}
	if want := []byte{commandIAC, commandDO, optionBinary}; !bytes.Equal(response, want) {
		t.Fatalf("response = %v, want %v", response, want)
	}
	if _, err := server.Write([]byte{'\r', 0, '\n'}); err != nil {
		t.Fatal(err)
	}
	got := <-remoteRead
	if got.err != nil || !bytes.Equal(got.data, []byte{'\r', 0, '\n'}) {
		t.Fatalf("application = %v, %v; want raw binary", got.data, got.err)
	}
}

func TestNegotiatesSupportedAndRefusesUnsupportedOptions(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	readResult := make(chan []byte, 1)
	go func() {
		value := make([]byte, 2)
		_, _ = io.ReadFull(connection, value)
		readResult <- value
	}()

	writeAndReadControl(t, server, []byte{commandIAC, commandWILL, optionEcho}, []byte{commandIAC, commandDO, optionEcho})
	const unsupported = byte(42)
	writeAndReadControl(t, server, []byte{commandIAC, commandWILL, unsupported}, []byte{commandIAC, commandDONT, unsupported})
	// A repeated hostile offer is ignored rather than creating an unbounded
	// negotiation response loop.
	if _, err := server.Write([]byte{commandIAC, commandWILL, unsupported, 'o', 'k'}); err != nil {
		t.Fatal(err)
	}
	if got := <-readResult; string(got) != "ok" {
		t.Fatalf("application data = %q, want ok", got)
	}
	if err := server.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if _, err := server.Read(buffer); err == nil {
		t.Fatal("repeated unsupported offer produced another response")
	}
}

func TestTerminalTypeAndNAWS(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{
		Address: "router", TerminalType: "VT100", WindowWidth: 255, WindowHeight: 24,
	})
	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 1)
		_, err := connection.Read(buffer)
		readDone <- err
	}()

	writeAndReadControl(t, server, []byte{commandIAC, commandDO, optionTerminalType}, []byte{commandIAC, commandWILL, optionTerminalType})
	writeAndReadControl(t, server,
		[]byte{commandIAC, commandSB, optionTerminalType, terminalTypeSEND, commandIAC, commandSE},
		[]byte{commandIAC, commandSB, optionTerminalType, terminalTypeIS, 'V', 'T', '1', '0', '0', commandIAC, commandSE},
	)
	if _, err := server.Write([]byte{commandIAC, commandDO, optionNAWS}); err != nil {
		t.Fatal(err)
	}
	want := []byte{
		commandIAC, commandWILL, optionNAWS,
		commandIAC, commandSB, optionNAWS, 0, commandIAC, commandIAC, 0, 24, commandIAC, commandSE,
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("NAWS = %v, want %v", got, want)
	}

	resizeDone := make(chan error, 1)
	go func() { resizeDone <- connection.SetWindowSize(132, 43) }()
	want = []byte{commandIAC, commandSB, optionNAWS, 0, 132, 0, 43, commandIAC, commandSE}
	got = make([]byte, len(want))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("resized NAWS = %v, want %v", got, want)
	}
	if err := <-resizeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := server.Write([]byte{'x'}); err != nil {
		t.Fatal(err)
	}
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
}

func TestSubnegotiationLimitAndMalformedInputCloseConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload []byte
		limit   int
		want    error
	}{
		{"oversized", []byte{commandIAC, commandSB, 42, 1, 2, 3, 4}, 3, ErrSubnegotiationTooLarge},
		{"malformed command", []byte{commandIAC, 100}, DefaultMaxSubnegotiationBytes, ErrMalformedNegotiation},
		{"malformed subnegotiation", []byte{commandIAC, commandSB, 42, 1, commandIAC, commandWILL}, DefaultMaxSubnegotiationBytes, ErrMalformedNegotiation},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection, server := pipeConnection(t, context.Background(), Config{Address: "router", MaxSubnegotiationBytes: test.limit})
			go func() { _, _ = server.Write(test.payload) }()
			_, err := connection.Read(make([]byte, 1))
			if !errors.Is(err, test.want) {
				t.Fatalf("Read() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadPreservesProtocolErrorAfterApplicationBytes(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	go func() { _, _ = server.Write([]byte{'x', commandIAC, 100}) }()
	buffer := make([]byte, 8)
	count, err := connection.Read(buffer)
	if err != nil || count != 1 || buffer[0] != 'x' {
		t.Fatalf("first Read = %d, %v, %q; want application byte", count, err, buffer[:count])
	}
	if _, err := connection.Read(buffer); !errors.Is(err, ErrMalformedNegotiation) {
		t.Fatalf("second Read error = %v, want malformed negotiation", err)
	}
}

func TestConcurrentCloseAndWritesAreSafe(t *testing.T) {
	connection, server := pipeConnection(t, context.Background(), Config{Address: "router"})
	drainDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, server)
		close(drainDone)
	}()
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				_, _ = connection.Write([]byte("show version\r"))
			}
		}()
	}
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_ = connection.Close()
		}()
	}
	group.Wait()
	_ = server.Close()
	<-drainDone
}

func TestCloseUnblocksAWriteWhenPeerStopsReading(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	signaling := &writeSignalingConn{Conn: client, started: make(chan struct{})}
	connection, err := (Dialer{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return signaling, nil
	}}).Dial(context.Background(), Config{Address: "router"})
	if err != nil {
		t.Fatal(err)
	}
	writeDone := make(chan error, 1)
	go func() {
		_, err := connection.Write([]byte("blocked"))
		writeDone <- err
	}()
	select {
	case <-signaling.started:
	case <-time.After(time.Second):
		t.Fatal("Write did not reach the raw connection")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- connection.Close() }()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind the active writer")
	}
	select {
	case err := <-writeDone:
		if err == nil {
			t.Fatal("blocked Write unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock Write")
	}
}

type writeSignalingConn struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (connection *writeSignalingConn) Write(buffer []byte) (int, error) {
	connection.once.Do(func() { close(connection.started) })
	return connection.Conn.Write(buffer)
}

func pipeConnection(t *testing.T, ctx context.Context, config Config) (*Conn, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	connection, err := (Dialer{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}}).Dial(ctx, config)
	if err != nil {
		_ = client.Close()
		_ = server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = server.Close()
	})
	return connection, server
}

func writeAndReadControl(t *testing.T, server net.Conn, sent, want []byte) {
	t.Helper()
	if _, err := server.Write(sent); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(server, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("control response = %v, want %v", got, want)
	}
}
