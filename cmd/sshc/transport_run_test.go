package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/serialtransport"
	"sshc/internal/telnet"
)

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type fakeDuplex struct {
	reader io.Reader
	mu     sync.Mutex
	writes bytes.Buffer
	closed bool
	close  func() error
}

type controlledDuplex struct {
	*fakeDuplex
	dtr      bool
	rts      bool
	breakFor time.Duration
}

func (stream *controlledDuplex) SetDTR(enabled bool) error { stream.dtr = enabled; return nil }
func (stream *controlledDuplex) SetRTS(enabled bool) error { stream.rts = enabled; return nil }
func (stream *controlledDuplex) Break(duration time.Duration) error {
	stream.breakFor = duration
	return nil
}

func (stream *fakeDuplex) Read(buffer []byte) (int, error) { return stream.reader.Read(buffer) }
func (stream *fakeDuplex) Write(buffer []byte) (int, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.writes.Write(buffer)
}
func (stream *fakeDuplex) Close() error {
	stream.mu.Lock()
	stream.closed = true
	closeStream := stream.close
	stream.mu.Unlock()
	if closeStream != nil {
		return closeStream()
	}
	return nil
}
func (stream *fakeDuplex) written() string {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.writes.String()
}

func TestRunTransportAutomationWritesCommandAndReportsMatch(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Target = "/dev/ttyUSB0"
	called.Command = []string{"show", "version"}
	called.Expect = `router# `
	called.JSON = true
	stream := &fakeDuplex{reader: strings.NewReader("version 1\nrouter# ")}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if got := stream.written(); got != "show version\r" {
		t.Fatalf("written = %q", got)
	}
	var report transportRunReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Success || !report.Matched || report.Transcript != "version 1\nrouter# " || report.TranscriptEncoding != "utf-8" {
		t.Fatalf("report = %#v", report)
	}
}

func TestOpenSerialAppliesRequestedModemControls(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, false)
	called.Target = "COM3"
	dtr, rts := false, true
	called.DTR, called.RTS, called.Break = &dtr, &rts, 250*time.Millisecond
	stream := &controlledDuplex{fakeDuplex: &fakeDuplex{reader: strings.NewReader("")}}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	opened, err := openTransport(context.Background(), called, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if stream.dtr || !stream.rts || stream.breakFor != 250*time.Millisecond {
		t.Fatalf("controls = dtr:%v rts:%v break:%s", stream.dtr, stream.rts, stream.breakFor)
	}
}

func TestRunTransportAutomationValidatesBeforeOpening(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Target = "/dev/ttyUSB0"
	called.Command = []string{"show"}
	called.Expect = "("
	called.JSON = true
	opened := false
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) {
			opened = true
			return nil, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if code != transportUsageExit || opened {
		t.Fatalf("code = %d, opened = %v", code, opened)
	}
	var report transportRunReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Success || report.Failure == nil || report.Failure.Kind != "invalid_script" {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunTransportAutomationRedactsEnvironmentSecret(t *testing.T) {
	t.Setenv("SSHC_TEST_CONSOLE_PASSWORD", "not-for-output")
	called := defaultTransportInvocation(transportTelnet, true)
	called.Target = "console.example"
	called.Script = "-"
	called.JSON = true
	stream := &fakeDuplex{reader: strings.NewReader("password: not-for-output\nrouter# ")}
	dependencies := transportDependencies{
		dialTelnet: func(context.Context, telnet.Config) (duplexStream, error) { return stream, nil },
	}
	document := `{"version":1,"steps":[{"sendEnv":"SSHC_TEST_CONSOLE_PASSWORD"},{"expect":"router# "}]}`
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, []string{telnetPlaintextWarning}, strings.NewReader(document), &stdout, &stderr, dependencies)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "not-for-output") || !strings.Contains(stdout.String(), "[REDACTED]") {
		t.Fatalf("secret was not redacted: %s", stdout.String())
	}
}

func TestRunTransportAutomationEncodesInvalidUTF8(t *testing.T) {
	called := defaultTransportInvocation(transportTelnet, true)
	called.Target = "console.example"
	called.Command = []string{"show"}
	called.Expect = `(?s)..`
	called.JSON = true
	stream := &fakeDuplex{reader: bytes.NewReader([]byte{0xff, 0x00})}
	dependencies := transportDependencies{
		dialTelnet: func(context.Context, telnet.Config) (duplexStream, error) { return stream, nil },
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	var report transportRunReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.TranscriptEncoding != "base64" || report.TranscriptBase64 != "/wA=" || report.Transcript != "" {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunTransportAutomationReturnsTimeoutCodeForReadFor(t *testing.T) {
	called := defaultTransportInvocation(transportTelnet, true)
	called.Target = "console.example"
	called.Command = []string{"show"}
	called.ReadFor = time.Second
	called.Timeout = 20 * time.Millisecond
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	stream := &fakeDuplex{reader: reader, close: reader.Close}
	dependencies := transportDependencies{
		dialTelnet: func(context.Context, telnet.Config) (duplexStream, error) { return stream, nil },
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if code != transportTimeoutExit || !strings.Contains(stderr.String(), "timeout") {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunSerialListJSONIsStable(t *testing.T) {
	dependencies := transportDependencies{
		listSerial: func(context.Context) ([]serialtransport.Device, error) {
			return []serialtransport.Device{{Name: "COM3"}, {Name: "COM8"}}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runSerialList(context.Background(), true, &stdout, &stderr, dependencies)
	if code != 0 || stderr.Len() != 0 || stdout.String() != "{\"schemaVersion\":1,\"devices\":[{\"name\":\"COM3\",\"isUsb\":false},{\"name\":\"COM8\",\"isUsb\":false}]}\n" {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestRunSerialListReportsOutputFailure(t *testing.T) {
	dependencies := transportDependencies{
		listSerial: func(context.Context) ([]serialtransport.Device, error) {
			return []serialtransport.Device{{Name: "COM3"}}, nil
		},
	}
	var stderr bytes.Buffer
	if code := runSerialList(context.Background(), false, failingOutput{}, &stderr, dependencies); code != transportFailureExit {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(stderr.String(), "could not write") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
