package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testTelnetWILL = byte(251)
	testTelnetDO   = byte(253)
	testTelnetIAC  = byte(255)
	testTelnetECHO = byte(1)
)

// TestRunTelnetEndToEnd uses the public argv shape, the production Telnet
// dialer, and a real loopback TCP connection. The tiny server deliberately
// splits its response so an incomplete chunk cannot satisfy the expectation.
func TestRunTelnetEndToEnd(t *testing.T) {
	listener := listenLoopback(t)
	serverErr := make(chan error, 1)
	firstResponseWritten := make(chan struct{})
	continueResponse := make(chan struct{})
	var releaseResponse sync.Once
	release := func() { releaseResponse.Do(func() { close(continueResponse) }) }
	defer release()
	go func() {
		serverErr <- serveTelnetConversation(listener, firstResponseWritten, continueResponse)
	}()

	address := listener.Addr().String()
	called, err := parseInvocation([]string{
		"sshc", "run", "telnet", address,
		"--connect-timeout", "2s",
		"--timeout", "2s",
		"--settle", "0",
		"--expect", `router# $`,
		"--json",
		"--", "show", "version",
	})
	if err != nil {
		t.Fatal(err)
	}
	if called.Kind != invocationRunTransport || called.Transport == nil {
		t.Fatalf("parsed invocation = %#v", called)
	}

	type commandResult struct {
		code   int
		stdout string
		stderr string
	}
	commandDone := make(chan commandResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		var stdout, stderr bytes.Buffer
		code := runTransportInvocation(ctx, *called.Transport, nil, &stdout, &stderr)
		commandDone <- commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
	}()

	select {
	case <-firstResponseWritten:
	case result := <-commandDone:
		t.Fatalf("command returned before the split response completed: %#v", result)
	case <-ctx.Done():
		t.Fatal("server did not receive the Telnet command")
	}
	select {
	case result := <-commandDone:
		t.Fatalf("partial response unexpectedly completed the command: %#v", result)
	default:
	}
	release()

	var result commandResult
	select {
	case result = <-commandDone:
	case <-ctx.Done():
		t.Fatal("Telnet command did not complete")
	}
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("code = %d, stderr = %q, stdout = %q", result.code, result.stderr, result.stdout)
	}
	var report transportRunReport
	if err := json.Unmarshal([]byte(result.stdout), &report); err != nil {
		t.Fatalf("decode JSON report: %v; output = %q", err, result.stdout)
	}
	if !report.Success || !report.Matched || report.Expectation != `router# $` || report.StepsCompleted != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Transport != transportTelnet || report.Target != address {
		t.Fatalf("report target = %q, transport = %q", report.Target, report.Transport)
	}
	if report.Transcript != "version 1\r\nrouter# " || report.TranscriptEncoding != "utf-8" {
		t.Fatalf("report transcript = %q (%s)", report.Transcript, report.TranscriptEncoding)
	}
	if len(report.Warnings) != 1 || report.Warnings[0] != telnetPlaintextWarning {
		t.Fatalf("report warnings = %#v", report.Warnings)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Telnet test server did not stop")
	}
}

func TestRunTelnetEndToEndTimesOutWhenPeerStopsResponding(t *testing.T) {
	listener := listenLoopback(t)
	serverErr := make(chan error, 1)
	commandReceived := make(chan struct{})
	releaseServer := make(chan struct{})
	var releasePeer sync.Once
	release := func() { releasePeer.Do(func() { close(releaseServer) }) }
	defer release()
	go func() {
		serverErr <- serveSilentTelnetPeer(listener, commandReceived, releaseServer)
	}()

	address := listener.Addr().String()
	called, err := parseInvocation([]string{
		"sshc", "run", "telnet", address,
		"--connect-timeout", "2s",
		"--timeout", "200ms",
		"--settle", "0",
		"--expect", `never arrives`,
		"--json",
		"--", "show", "clock",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := runTransportInvocation(ctx, *called.Transport, nil, &stdout, &stderr)
	release()
	if code != transportTimeoutExit || stderr.Len() != 0 {
		t.Fatalf("code = %d, stderr = %q, stdout = %q", code, stderr.String(), stdout.String())
	}
	select {
	case <-commandReceived:
	default:
		t.Fatal("server did not receive the command before the client timed out")
	}
	var report transportRunReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Success || report.Failure == nil || report.Failure.Kind != "timeout" || report.Failure.Step != 2 {
		t.Fatalf("report = %#v", report)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("silent Telnet test server did not stop")
	}
}

func listenLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func serveTelnetConversation(listener net.Listener, firstWritten chan<- struct{}, continueResponse <-chan struct{}) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if err := negotiateEcho(connection); err != nil {
		return err
	}
	command, err := readThrough(connection, "\r\n", 1<<10)
	if err != nil {
		return err
	}
	if command != "show version\r\n" {
		return fmt.Errorf("Telnet command = %q", command)
	}
	if err := writeAllForTest(connection, []byte("ver")); err != nil {
		return err
	}
	close(firstWritten)
	<-continueResponse
	return writeAllForTest(connection, []byte("sion 1\r\nrouter# "))
}

func serveSilentTelnetPeer(listener net.Listener, commandReceived chan<- struct{}, release <-chan struct{}) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	if err := negotiateEcho(connection); err != nil {
		return err
	}
	command, err := readThrough(connection, "\r\n", 1<<10)
	if err != nil {
		return err
	}
	if command != "show clock\r\n" {
		return fmt.Errorf("Telnet command = %q", command)
	}
	close(commandReceived)
	<-release
	return nil
}

func negotiateEcho(connection net.Conn) error {
	if err := writeAllForTest(connection, []byte{testTelnetIAC, testTelnetWILL, testTelnetECHO}); err != nil {
		return err
	}
	reply := make([]byte, 3)
	if _, err := io.ReadFull(connection, reply); err != nil {
		return err
	}
	want := []byte{testTelnetIAC, testTelnetDO, testTelnetECHO}
	if !bytes.Equal(reply, want) {
		return fmt.Errorf("Telnet negotiation reply = %v, want %v", reply, want)
	}
	return nil
}

func readThrough(reader io.Reader, suffix string, limit int) (string, error) {
	var collected strings.Builder
	buffer := make([]byte, 1)
	for collected.Len() < limit {
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return "", err
		}
		collected.WriteByte(buffer[0])
		if strings.HasSuffix(collected.String(), suffix) {
			return collected.String(), nil
		}
	}
	return "", fmt.Errorf("input exceeded %d bytes", limit)
}

func writeAllForTest(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}
