package streamrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedStream struct {
	reader io.Reader
	mu     sync.Mutex
	writes bytes.Buffer
}

type blockingWriterStream struct {
	reader  io.Reader
	writing chan struct{}
	release chan struct{}
}

type settlingReaderStream struct {
	mu      sync.Mutex
	chunks  [][]byte
	timeout time.Duration
}

type contextDiscardingStream struct{}

func (*contextDiscardingStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (*contextDiscardingStream) Write(buffer []byte) (int, error) { return len(buffer), nil }
func (*contextDiscardingStream) DiscardPending(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (stream *settlingReaderStream) SetReadTimeout(timeout time.Duration) error {
	stream.mu.Lock()
	stream.timeout = timeout
	stream.mu.Unlock()
	return nil
}

func (stream *settlingReaderStream) Read(buffer []byte) (int, error) {
	stream.mu.Lock()
	if len(stream.chunks) > 0 {
		chunk := stream.chunks[0]
		stream.chunks = stream.chunks[1:]
		stream.mu.Unlock()
		return copy(buffer, chunk), nil
	}
	timeout := stream.timeout
	stream.mu.Unlock()
	if timeout > 0 {
		time.Sleep(timeout)
		return 0, nil
	}
	return 0, io.EOF
}

func (stream *settlingReaderStream) Write(buffer []byte) (int, error) { return len(buffer), nil }

func (s *blockingWriterStream) Read(buffer []byte) (int, error) { return s.reader.Read(buffer) }
func (s *blockingWriterStream) Write(buffer []byte) (int, error) {
	close(s.writing)
	<-s.release
	return len(buffer), nil
}

func (s *scriptedStream) Read(buffer []byte) (int, error) { return s.reader.Read(buffer) }
func (s *scriptedStream) Write(buffer []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes.Write(buffer)
}
func (s *scriptedStream) written() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes.String()
}

func ptr(value string) *string { return &value }

func TestRunSendsThenMatchesWithoutLosingChunkSuffix(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("banner\nrouter# next> ")}
	script := Script{Steps: []Step{
		{Send: ptr("show version"), LineEnding: EndingCR},
		{Expect: `router# `, LineEnding: EndingNone},
		{Expect: `next> `, LineEnding: EndingNone},
	}}
	result, err := Run(context.Background(), stream, script, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stream.written(); got != "show version\r" {
		t.Fatalf("written = %q", got)
	}
	if got := string(result.Transcript); got != "banner\nrouter# next> " {
		t.Fatalf("transcript = %q", got)
	}
	if result.StepsCompleted != 3 || !result.Matched || result.LastExpectation != `next> ` {
		t.Fatalf("result = %#v", result)
	}
}

type chunkReader struct {
	mu     sync.Mutex
	chunks [][]byte
}

func (reader *chunkReader) Read(buffer []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := reader.chunks[0]
	reader.chunks = reader.chunks[1:]
	return copy(buffer, chunk), nil
}

func TestRunDoesNotMatchPromptObservedBeforeSend(t *testing.T) {
	reader := &chunkReader{chunks: [][]byte{[]byte("router# stale"), []byte("command output\nrouter# ")}}
	stream := &scriptedStream{reader: reader}
	script := Script{Steps: []Step{
		{Expect: `router# `, LineEnding: EndingNone},
		{Send: ptr("show version"), LineEnding: EndingCR},
		{Expect: `router# `, LineEnding: EndingNone},
	}}
	result, err := Run(context.Background(), stream, script, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := string(result.Transcript)
	if result.StepsCompleted != 3 || got != "router# stalecommand output\nrouter# " {
		t.Fatalf("result = %#v, transcript = %q", result, got)
	}
}

func TestRunTimesOutBlockedRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	stream := &scriptedStream{reader: reader}
	started := time.Now()
	_, err := Run(context.Background(), stream, Script{Steps: []Step{{Expect: "never", LineEnding: EndingNone}}}, Options{Timeout: 20 * time.Millisecond})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureTimeout {
		t.Fatalf("error = %#v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func TestRunTotalTimeoutAlsoBoundsReadFor(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	stream := &scriptedStream{reader: reader}
	started := time.Now()
	_, err := Run(context.Background(), stream, Script{Steps: []Step{{ReadFor: time.Second, LineEnding: EndingNone}}}, Options{Timeout: 20 * time.Millisecond})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureTimeout {
		t.Fatalf("error = %#v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func TestRunTimesOutBlockedWrite(t *testing.T) {
	stream := &blockingWriterStream{
		reader: strings.NewReader(""), writing: make(chan struct{}), release: make(chan struct{}),
	}
	done := make(chan error, 1)
	go func() {
		_, err := Run(context.Background(), stream, Script{Steps: []Step{{Send: ptr("show"), LineEnding: EndingCR}}}, Options{Timeout: 20 * time.Millisecond})
		done <- err
	}()
	<-stream.writing
	err := <-done
	close(stream.release)
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureTimeout {
		t.Fatalf("error = %#v", err)
	}
}

func TestRunTotalTimeoutBoundsStaleInputDrain(t *testing.T) {
	started := time.Now()
	_, err := Run(context.Background(), &contextDiscardingStream{}, Script{Steps: []Step{{
		Send: ptr("show"), LineEnding: EndingCR,
	}}}, Options{Timeout: 20 * time.Millisecond})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureTimeout {
		t.Fatalf("error = %#v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func TestRunStopsAtOutputLimit(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("0123456789")}
	result, err := Run(context.Background(), stream, Script{Steps: []Step{{Expect: "missing", LineEnding: EndingNone}}}, Options{MaxBytes: 5})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureOutputLimit {
		t.Fatalf("error = %#v", err)
	}
	if got := string(result.Transcript); got != "01234" {
		t.Fatalf("transcript = %q", got)
	}
}

func TestRunAcceptsExpectationBeforeOutputLimitInSameRead(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("ok>xxxxxxxx")}
	result, err := Run(context.Background(), stream, Script{Steps: []Step{{Expect: `ok>`, LineEnding: EndingNone}}}, Options{MaxBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Matched || string(result.Transcript) != "ok>xx" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunRequiresQuietAfterPromptShapedMatch(t *testing.T) {
	stream := &settlingReaderStream{chunks: [][]byte{
		[]byte("banner\nSwitch#"),
		[]byte("\nstill output\nSwitch#"),
	}}
	result, err := Run(context.Background(), stream, Script{Steps: []Step{{
		Expect: `(?m)^Switch#$`, LineEnding: EndingNone,
	}}}, Options{Timeout: time.Second, Settle: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(result.Transcript); got != "banner\nSwitch#\nstill output\nSwitch#" {
		t.Fatalf("transcript = %q", got)
	}
}

func TestRunReadsEnvironmentAndRedactsEcho(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("password: secret-value\n# ")}
	result, err := Run(context.Background(), stream, Script{Steps: []Step{
		{SendEnv: "DEVICE_PASSWORD", LineEnding: EndingCR},
		{Expect: `# `, LineEnding: EndingNone},
	}}, Options{LookupEnv: func(name string) (string, bool) {
		if name != "DEVICE_PASSWORD" {
			return "", false
		}
		return "secret-value", true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := stream.written(); got != "secret-value\r" {
		t.Fatalf("written = %q", got)
	}
	redacted := string(Redact(result.Transcript, result.Secrets))
	if strings.Contains(redacted, "secret-value") || redacted != "password: [REDACTED]\n# " {
		t.Fatalf("redacted = %q", redacted)
	}
}

func TestRedactMasksSecretPrefixAtTruncatedBoundary(t *testing.T) {
	redacted := Redact([]byte("password: not-for-"), []Secret{{Value: []byte("not-for-output")}})
	if got := string(redacted); got != "password: [REDACTED]" {
		t.Fatalf("redacted = %q", got)
	}
}

func TestRedactDoesNotCorruptOrdinaryOutputThatStartsLikeASecret(t *testing.T) {
	redacted := Redact([]byte("password: not-for-X\nready"), []Secret{{
		Value: []byte("not-for-output"), TranscriptStart: len("password: "),
	}})
	if got := string(redacted); got != "password: not-for-X\nready" {
		t.Fatalf("redacted = %q", got)
	}
}

func TestRedactDoesNotMaskOneBytePrefixInNormalResponse(t *testing.T) {
	redacted := Redact([]byte("shell ready\n"), []Secret{{Value: []byte("show-secret")}})
	if got := string(redacted); got != "shell ready\n" {
		t.Fatalf("redacted = %q", got)
	}
}

func TestRunValidatesEveryStepBeforeWriting(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("")}
	_, err := Run(context.Background(), stream, Script{Steps: []Step{
		{Send: ptr("must-not-send"), LineEnding: EndingCR},
		{Expect: "(", LineEnding: EndingNone},
	}}, Options{})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureInvalid {
		t.Fatalf("error = %#v", err)
	}
	if got := stream.written(); got != "" {
		t.Fatalf("wrote %q before validation completed", got)
	}
}

func TestRunReadForRejectsEarlyEOFAndKeepsOutput(t *testing.T) {
	stream := &scriptedStream{reader: strings.NewReader("answer")}
	result, err := Run(context.Background(), stream, Script{Steps: []Step{{ReadFor: time.Second, LineEnding: EndingNone}}}, Options{})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureRead {
		t.Fatalf("error = %#v", err)
	}
	if got := string(result.Transcript); got != "answer" {
		t.Fatalf("transcript = %q", got)
	}
}

func TestValidateRequiresReadForToBeFinal(t *testing.T) {
	err := Validate(Script{Steps: []Step{
		{ReadFor: time.Millisecond, LineEnding: EndingNone},
		{Expect: "later", LineEnding: EndingNone},
	}}, Options{})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureInvalid {
		t.Fatalf("error = %#v", err)
	}
}
