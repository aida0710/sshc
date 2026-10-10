package sshclient_test

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// Network tests must fail with a useful assertion instead of hanging the suite.
const remoteForwardTestDeadline = 5 * time.Second

type remoteForwardSettings struct {
	specs        []sshclient.ForwardSpec
	deny         bool
	origin       string
	ignoreListen bool
	ignoreCancel bool
}

type remoteForwardFixture struct {
	settings          remoteForwardSettings
	mutex             sync.Mutex
	listeners         map[string]net.Listener
	reservedListeners map[string]net.Listener
	requested         chan string
}

func remoteForwardSession(t *testing.T, settings remoteForwardSettings) (terminal.Process, *remoteForwardFixture) {
	t.Helper()
	fixture := &remoteForwardFixture{
		settings: settings, listeners: make(map[string]net.Listener),
		reservedListeners: make(map[string]net.Listener), requested: make(chan string, 4),
	}
	t.Cleanup(func() { fixture.close() })
	for index := range fixture.settings.specs {
		if fixture.settings.specs[index].ListenPort == "" {
			fixture.settings.specs[index].ListenPort = fixture.reservePort(t)
		}
	}
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys:      []ssh.PublicKey{public},
		OnGlobalRequest: fixture.handle,
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "ready\r\n")
			_, _ = io.Copy(io.Discard, channel)
		},
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.Forwards = fixture.settings.specs
	process, err := dialerFor(t, server, auth).Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	readUntil(t, process, "ready")
	return process, fixture
}

func (fixture *remoteForwardFixture) reservePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		t.Fatal(err)
	}
	// Keep the listener open until the SSH request claims it. Closing a
	// "free" port first lets another fixture or process occupy that address.
	fixture.mutex.Lock()
	fixture.reservedListeners[listener.Addr().String()] = listener
	fixture.mutex.Unlock()
	return port
}

func (fixture *remoteForwardFixture) close() {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	for _, listener := range fixture.listeners {
		_ = listener.Close()
	}
	for _, listener := range fixture.reservedListeners {
		_ = listener.Close()
	}
}

func (fixture *remoteForwardFixture) handle(connection ssh.Conn, request *ssh.Request) bool {
	if request.Type != "tcpip-forward" && request.Type != "cancel-tcpip-forward" {
		return false
	}
	var address struct {
		Host string
		Port uint32
	}
	if err := ssh.Unmarshal(request.Payload, &address); err != nil {
		_ = request.Reply(false, nil)
		return true
	}
	name := net.JoinHostPort(address.Host, strconv.Itoa(int(address.Port)))
	if request.Type == "cancel-tcpip-forward" {
		fixture.mutex.Lock()
		listener := fixture.listeners[name]
		delete(fixture.listeners, name)
		fixture.mutex.Unlock()
		if listener != nil {
			_ = listener.Close()
		}
		if !fixture.settings.ignoreCancel {
			_ = request.Reply(listener != nil, nil)
		}
		return true
	}
	fixture.requested <- name
	if fixture.settings.ignoreListen {
		return true
	}
	if fixture.settings.deny {
		_ = request.Reply(false, nil)
		return true
	}
	fixture.mutex.Lock()
	listener := fixture.reservedListeners[name]
	delete(fixture.reservedListeners, name)
	if listener != nil {
		fixture.listeners[name] = listener
	}
	fixture.mutex.Unlock()
	if listener == nil {
		_ = request.Reply(false, nil)
		return true
	}
	_ = request.Reply(true, nil)
	go func() {
		_ = connection.Wait()
		_ = listener.Close()
	}()
	go fixture.accept(connection, listener)
	return true
}

func (fixture *remoteForwardFixture) accept(connection ssh.Conn, listener net.Listener) {
	for {
		incoming, err := listener.Accept()
		if err != nil {
			return
		}
		go fixture.forward(connection, incoming)
	}
}

func (fixture *remoteForwardFixture) forward(connection ssh.Conn, incoming net.Conn) {
	defer func() { _ = incoming.Close() }()
	host, port, _ := net.SplitHostPort(incoming.LocalAddr().String())
	origin, originPort, _ := net.SplitHostPort(incoming.RemoteAddr().String())
	if fixture.settings.origin != "" {
		origin = fixture.settings.origin
	}
	listenPort, _ := strconv.Atoi(port)
	clientPort, _ := strconv.Atoi(originPort)
	payload := struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}{host, uint32(listenPort), origin, uint32(clientPort)}
	channel, requests, err := connection.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))
	if err != nil {
		return
	}
	defer func() { _ = channel.Close() }()
	go ssh.DiscardRequests(requests)
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(channel, incoming)
		_ = channel.CloseWrite()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(incoming, channel)
		_ = incoming.(*net.TCPConn).CloseWrite()
		done <- struct{}{}
	}()
	<-done
	<-done
}

func remoteForwardDial(t *testing.T, address string) net.Conn {
	t.Helper()
	connection, err := net.DialTimeout("tcp", address, remoteForwardTestDeadline)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	_ = connection.SetDeadline(time.Now().Add(remoteForwardTestDeadline))
	return connection
}

func TestRemoteForwardCarriesBytesFromTheSSHServerToTheEngineDestination(t *testing.T) {
	destination := echoServer(t)
	process, fixture := remoteForwardSession(t, remoteForwardSettings{specs: []sshclient.ForwardSpec{{Kind: terminal.ForwardRemote, Requested: "0.0.0.0", To: destination}}})
	port := fixture.settings.specs[0].ListenPort
	if requested := <-fixture.requested; requested != "127.0.0.1:"+port {
		t.Fatalf("remote listener = %q", requested)
	}
	forward := process.(terminal.Forwarder).Forwards()[0]
	if forward.Kind != "remote" || forward.Problem != "" || forward.Temporary {
		t.Fatalf("forward = %#v", forward)
	}
	connection := remoteForwardDial(t, forward.Listen)
	_, _ = io.WriteString(connection, "remote tunnel")
	answer := make([]byte, len("echo:remote tunnel"))
	if _, err := io.ReadFull(connection, answer); err != nil {
		t.Fatal(err)
	}
	if string(answer) != "echo:remote tunnel" {
		t.Fatalf("answer = %q", answer)
	}
}

func TestStoppingRemoteForwardClosesItsActiveTunnelsAndKeepsTheSession(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			accepted <- connection
		}
	}()
	process, fixture := remoteForwardSession(t, remoteForwardSettings{})
	controller := process.(terminal.ForwardController)
	forward, err := controller.StartForward(terminal.ForwardRemote, fixture.reservePort(t), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	connection := remoteForwardDial(t, forward.Listen)
	var destination net.Conn
	select {
	case destination = <-accepted:
	case <-time.After(remoteForwardTestDeadline):
		t.Fatal("engine destination was never reached")
	}
	t.Cleanup(func() { _ = destination.Close() })
	if err := controller.StopForward(forward.ID); err != nil {
		t.Fatal(err)
	}
	_ = destination.SetReadDeadline(time.Now().Add(remoteForwardTestDeadline))
	if _, err := destination.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("destination after stop = %v", err)
	}
	if _, err := connection.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("remote peer after stop = %v", err)
	}
	if got := process.(terminal.Forwarder).Forwards(); len(got) != 0 {
		t.Fatalf("forwards after stop = %#v", got)
	}
	if _, err := process.Write([]byte("still connected\r")); err != nil {
		t.Fatal(err)
	}
	if reopened, err := net.Listen("tcp", forward.Listen); err != nil {
		t.Fatal(err)
	} else {
		_ = reopened.Close()
	}
}

func TestRemoteForwardDenialIsVisibleWithoutEndingTheSession(t *testing.T) {
	process, fixture := remoteForwardSession(t, remoteForwardSettings{deny: true, specs: []sshclient.ForwardSpec{{Kind: terminal.ForwardRemote, To: "127.0.0.1:80"}}})
	forwards := process.(terminal.Forwarder).Forwards()
	if len(forwards) != 1 || forwards[0].Problem != terminal.ForwardProblemRemoteDenied {
		t.Fatalf("forwards = %#v", forwards)
	}
	forward, err := process.(terminal.ForwardController).StartForward(terminal.ForwardRemote, fixture.reservePort(t), "127.0.0.1:80")
	if err == nil || forward.Problem != terminal.ForwardProblemRemoteDenied {
		t.Fatalf("temporary forward = %#v, %v", forward, err)
	}
	if len(process.(terminal.Forwarder).Forwards()) != 1 {
		t.Fatal("failed temporary listener was retained")
	}
	if _, err := process.Write([]byte("still connected\r")); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteForwardRejectsExternallyOriginatedConnections(t *testing.T) {
	process, fixture := remoteForwardSession(t, remoteForwardSettings{origin: "203.0.113.10"})
	port := fixture.reservePort(t)
	if competitor, err := net.Listen("tcp", "127.0.0.1:"+port); err == nil {
		_ = competitor.Close()
		t.Fatal("remote listener reservation was released before its SSH request")
	}
	destination := echoServer(t)
	forward, err := process.(terminal.ForwardController).StartForward(terminal.ForwardRemote, port, destination)
	if err != nil {
		t.Fatal(err)
	}
	connection := remoteForwardDial(t, forward.Listen)
	_, _ = io.WriteString(connection, "must not reach engine")
	if _, err := connection.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("external connection = %v", err)
	}
}

func TestClosingTheSessionDoesNotWaitForRemoteListenerCancellation(t *testing.T) {
	process, fixture := remoteForwardSession(t, remoteForwardSettings{ignoreCancel: true})
	port := fixture.reservePort(t)
	destination := echoServer(t)
	if _, err := process.(terminal.ForwardController).StartForward(terminal.ForwardRemote, port, destination); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- process.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(remoteForwardTestDeadline):
		t.Fatal("session close waited on remote cancellation reply")
	}
}

func TestClosingTheSessionCancelsAPendingRemoteListenRequest(t *testing.T) {
	process, fixture := remoteForwardSession(t, remoteForwardSettings{ignoreListen: true})
	started := make(chan error, 1)
	port := fixture.reservePort(t)
	go func() {
		_, err := process.(terminal.ForwardController).StartForward(terminal.ForwardRemote, port, "127.0.0.1:80")
		started <- err
	}()
	select {
	case <-fixture.requested:
	case <-time.After(remoteForwardTestDeadline):
		t.Fatal("listen request was never received")
	}
	_ = process.Close()
	select {
	case err := <-started:
		if err == nil {
			t.Fatal("cancelled remote request succeeded")
		}
	case <-time.After(remoteForwardTestDeadline):
		t.Fatal("pending remote listener survived session close")
	}
}

func TestRemoteForwardRequiresAPortAndAnExplicitEngineDestination(t *testing.T) {
	for _, value := range []string{"8080 127.0.0.1:80", "[::1]:8080 [::1]:80"} {
		spec, err := sshclient.ParseRemoteForward(value)
		if err != nil || spec.Kind != "remote" {
			t.Fatalf("ParseRemoteForward(%q) = %#v, %v", value, spec, err)
		}
	}
	for _, value := range []string{"8080", "0 127.0.0.1:80", "8080 :80", "8080 localhost:0", "8080 localhost:65536", "8080 localhost:http"} {
		if _, err := sshclient.ParseRemoteForward(value); err == nil {
			t.Errorf("invalid remote forward accepted: %q", value)
		}
	}
}

func TestRemoteForwardDeliversTheResponseAfterTheCallerHalfCloses(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_ = connection.SetDeadline(time.Now().Add(remoteForwardTestDeadline))
		contents, err := io.ReadAll(connection)
		if err == nil {
			_, _ = connection.Write(append([]byte("after EOF:"), contents...))
		}
	}()
	process, fixture := remoteForwardSession(t, remoteForwardSettings{})
	forward, err := process.(terminal.ForwardController).StartForward(terminal.ForwardRemote, fixture.reservePort(t), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	connection := remoteForwardDial(t, forward.Listen)
	_, _ = io.WriteString(connection, "request")
	if err := connection.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	answer, err := io.ReadAll(connection)
	if err != nil || string(answer) != "after EOF:request" {
		t.Fatalf("half-close response = %q, %v", answer, err)
	}
}

func TestRemoteForwardRejectsConnectionsAboveItsLimit(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var accepted atomic.Int64
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() {
				defer func() { _ = connection.Close() }()
				_, _ = connection.Write([]byte{1})
				_, _ = io.Copy(io.Discard, connection)
			}()
		}
	}()
	process, fixture := remoteForwardSession(t, remoteForwardSettings{})
	controller := process.(terminal.ForwardController)
	forward, err := controller.StartForward(terminal.ForwardRemote, fixture.reservePort(t), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// Hold 64 active tunnels; the next connection must be rejected before a
	// local destination or additional worker is created.
	for range 64 {
		connection := remoteForwardDial(t, forward.Listen)
		if _, err := io.ReadFull(connection, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
	}
	rejected := remoteForwardDial(t, forward.Listen)
	if _, err := rejected.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("connection above limit = %v", err)
	}
	if accepted.Load() != 64 {
		t.Fatalf("local destinations reached = %d", accepted.Load())
	}
	if err := controller.StopForward(forward.ID); err != nil {
		t.Fatal(err)
	}
}
