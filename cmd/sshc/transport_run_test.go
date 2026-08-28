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
	"sshc/internal/streamrun"
	"sshc/internal/telnet"
	"sshc/internal/textencoding"
)

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type blockingWriter struct {
	release <-chan struct{}
}

func (writer blockingWriter) Write(buffer []byte) (int, error) {
	<-writer.release
	return len(buffer), nil
}

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

func TestRunTransportAutomationConvertsLegacyEncodingAtTheStreamBoundary(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Target = "/dev/ttyUSB0"
	called.Encoding = textencoding.ShiftJIS
	called.Command = []string{"送信"}
	called.Expect = `受信# `
	stream := &fakeDuplex{reader: bytes.NewReader([]byte{0x8e, 0xf3, 0x90, 0x4d, '#', ' '})}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if code != 0 || stderr.Len() != 0 || stdout.String() != "受信# " {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	want := []byte{0x91, 0x97, 0x90, 0x4d, '\r'}
	if got := []byte(stream.written()); !bytes.Equal(got, want) {
		t.Fatalf("wire output = %x, want %x", got, want)
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

func TestRunTransportAutomationSendsExplicitFailureCleanup(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Target = "/dev/ttyUSB0"
	called.Script = "-"
	called.JSON = true
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	stream := &fakeDuplex{reader: reader, close: reader.Close}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	document := `{
		"version": 1,
		"steps": [
			{"send": "show version"},
			{"expect": "router#", "timeout": "10ms"}
		],
		"onFailure": {"send": "\u0003", "lineEnding": "none", "timeout": "100ms"}
	}`
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(document), &stdout, &stderr, dependencies)
	var report struct {
		Success        bool `json:"success"`
		FailureCleanup *struct {
			Attempted bool `json:"attempted"`
			Success   bool `json:"success"`
		} `json:"failureCleanup"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != transportTimeoutExit || stderr.Len() != 0 || report.Success || report.FailureCleanup == nil || !report.FailureCleanup.Attempted || !report.FailureCleanup.Success {
		t.Fatalf("code = %d, stderr = %q, report = %#v", code, stderr.String(), report)
	}
	if got := stream.written(); got != "show version\r\x03" {
		t.Fatalf("written = %q", got)
	}
}

func TestRunTransportAutomationSkipsFailureCleanupOnSuccess(t *testing.T) {
	called := defaultTransportInvocation(transportSerial, true)
	called.Target = "/dev/ttyUSB0"
	called.Script = "-"
	called.JSON = true
	called.Settle = 0
	stream := &fakeDuplex{reader: strings.NewReader("router#")}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	document := `{
		"version": 1,
		"steps": [
			{"send": "show version"},
			{"expect": "router#"}
		],
		"onFailure": {"send": "\u0003", "lineEnding": "none", "timeout": "100ms"}
	}`
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), called, nil, strings.NewReader(document), &stdout, &stderr, dependencies)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	if got := stream.written(); got != "show version\r" {
		t.Fatalf("written = %q", got)
	}
	if strings.Contains(stdout.String(), "failureCleanup") {
		t.Fatalf("success report contains failure cleanup: %s", stdout.String())
	}
}

func TestRunTransportFailureCleanupIsBounded(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := time.Now()
	report := runTransportFailureCleanup(context.Background(), blockingWriter{release: release}, transportFailureCleanup{
		Send: "q", LineEnding: streamrun.EndingNone, Timeout: 10 * time.Millisecond,
	})
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("cleanup took %s", elapsed)
	}
	if !report.Attempted || report.Success || report.Error != "cleanup write timed out" {
		t.Fatalf("report = %#v", report)
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

func TestRequireOutputRejectsSilentReadFor(t *testing.T) {
	called, err := parseInvocation([]string{
		"sshc", "serial", "/dev/ttyUSB0", "--non-interactive",
		"--require-output", "--read-for", "10ms", "--json", "--", "show", "clock",
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	stream := &fakeDuplex{reader: reader, close: reader.Close}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), *called.Transport, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	var report struct {
		Success       bool `json:"success"`
		BytesReceived int  `json:"bytesReceived"`
		Failure       *struct {
			Kind string `json:"kind"`
		} `json:"failure"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != transportFailureExit || stderr.Len() != 0 || report.Success || report.BytesReceived != 0 || report.Failure == nil || report.Failure.Kind != "no_output" {
		t.Fatalf("code = %d, stderr = %q, report = %#v", code, stderr.String(), report)
	}
}

func TestRequireOutputAcceptsReadForResponse(t *testing.T) {
	called, err := parseInvocation([]string{
		"sshc", "serial", "/dev/ttyUSB0", "--non-interactive",
		"--require-output", "--read-for", "20ms", "--json", "--", "show", "clock",
	})
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	stream := &fakeDuplex{reader: reader, close: reader.Close}
	dependencies := transportDependencies{
		openSerial: func(context.Context, serialtransport.Config) (duplexStream, error) { return stream, nil },
	}
	written := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("ready"))
		written <- err
	}()
	var stdout, stderr bytes.Buffer
	code := runTransportAutomation(context.Background(), *called.Transport, nil, strings.NewReader(""), &stdout, &stderr, dependencies)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	var report struct {
		Success       bool `json:"success"`
		BytesReceived int  `json:"bytesReceived"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if code != 0 || stderr.Len() != 0 || !report.Success || report.BytesReceived != len("ready") {
		t.Fatalf("code = %d, stderr = %q, report = %#v", code, stderr.String(), report)
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

func TestRunSerialListHumanOutputIdentifiesUSBDevices(t *testing.T) {
	dependencies := transportDependencies{
		listSerial: func(context.Context) ([]serialtransport.Device, error) {
			return []serialtransport.Device{
				{Name: "/dev/cu.debug-console"},
				{
					Name: "/dev/cu.usbserial-A5069RR4", USB: true,
					VID: "0403", PID: "6001", SerialNumber: "A5069RR4",
					Product: "FT232R USB UART", Manufacturer: "FTDI",
				},
			}, nil
		},
	}
	var stdout, stderr bytes.Buffer
	code := runSerialList(context.Background(), false, &stdout, &stderr, dependencies)
	want := "/dev/cu.debug-console\n" +
		"/dev/cu.usbserial-A5069RR4  [USB 0403:6001]  FTDI / FT232R USB UART  [serial A5069RR4]\n"
	if code != 0 || stderr.Len() != 0 || stdout.String() != want {
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
