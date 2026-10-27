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
func (*contextDiscardingStream) DiscardPending(ctx context.Context, _ time.Duration) error {
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

func TestRedactHidesALongerSecretThatStartsWithAnotherInEitherSendOrder(t *testing.T) {
	transcript := []byte("login: hunter2\nlater: hunter2-admin-9f\n")
	short := Secret{Value: []byte("hunter2"), TranscriptStart: 0}
	long := Secret{Value: []byte("hunter2-admin-9f"), TranscriptStart: 0}
	for name, secrets := range map[string][]Secret{
		"short sent first": {short, long},
		"long sent first":  {long, short},
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(Redact(transcript, secrets)); got != "login: [REDACTED]\nlater: [REDACTED]\n" {
				t.Fatalf("redacted = %q", got)
			}
		})
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

type failingWriterStream struct{}

func (failingWriterStream) Read([]byte) (int, error)  { return 0, io.EOF }
func (failingWriterStream) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestRunReportsStreamFailureRatherThanTimeoutWhenStepHasTimeout(t *testing.T) {
	cases := []struct {
		name   string
		stream io.ReadWriter
		step   Step
		want   FailureKind
	}{
		{
			name:   "expect sees EOF",
			stream: &scriptedStream{reader: strings.NewReader("")},
			step:   Step{Expect: "prompt> ", LineEnding: EndingNone, Timeout: time.Second},
			want:   FailureRead,
		},
		{
			name:   "readFor sees EOF",
			stream: &scriptedStream{reader: strings.NewReader("partial")},
			step:   Step{ReadFor: time.Second, LineEnding: EndingNone, Timeout: time.Second},
			want:   FailureRead,
		},
		{
			name:   "send fails to write",
			stream: failingWriterStream{},
			step:   Step{Send: ptr("show"), LineEnding: EndingCR, Timeout: time.Second},
			want:   FailureWrite,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Run(context.Background(), testCase.stream, Script{Steps: []Step{testCase.step}}, Options{Timeout: 2 * time.Second})
			var failure *Error
			if !errors.As(err, &failure) || failure.Kind != testCase.want {
				t.Fatalf("error = %#v, want kind %q", err, testCase.want)
			}
		})
	}
}

func TestRunReportsStepTimeoutWhenStepDeadlinePassesFirst(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	stream := &scriptedStream{reader: reader}
	_, err := Run(context.Background(), stream, Script{Steps: []Step{{
		Expect: "never", LineEnding: EndingNone, Timeout: 20 * time.Millisecond,
	}}}, Options{Timeout: time.Second})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != FailureTimeout {
		t.Fatalf("error = %#v", err)
	}
}

// heldChildCancellationContext は、取り消しを子の context へすぐには伝えない呼び出し元の
// context である。AfterFunc を持つ親に対して、context.WithTimeout は子の取り消しをその
// AfterFunc に任せるので、release を呼ぶまで Run の期限は取り消されない。
//
// 本番の Telnet と Serial は、呼び出し元の context の AfterFunc で接続を閉じる。取り消しが
// その AfterFunc と Run の期限へ順に伝わる間に、閉じた接続の読み込みの失敗が先に Run へ
// 届くことがある。この順番を毎回作る。
type heldChildCancellationContext struct {
	context.Context
	mu   sync.Mutex
	held []func()
}

func (ctx *heldChildCancellationContext) AfterFunc(cancelChild func()) (stop func() bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.held = append(ctx.held, cancelChild)
	return func() bool { return false }
}

// Value は何も返さない。埋め込んだ context の値を見せると、context.WithTimeout は
// AfterFunc を使わず、標準の取り消しの仕組みで子を直接つなぐ。
func (*heldChildCancellationContext) Value(any) any { return nil }

func (ctx *heldChildCancellationContext) release() {
	ctx.mu.Lock()
	held := ctx.held
	ctx.held = nil
	ctx.mu.Unlock()
	for _, cancelChild := range held {
		cancelChild()
	}
}

// errClosedConnection は、閉じたソケットの読み込みが返す誤りの代わり。
var errClosedConnection = errors.New("use of closed network connection")

// closedOnCancelStream は、呼び出し元が取り消されるとソケットを閉じる Telnet の接続を
// まねる。output を返し終えたあとの読み込みは閉じられるまで止まり、閉じたら失敗する。
type closedOnCancelStream struct {
	mu          sync.Mutex
	output      []byte
	closed      <-chan struct{}
	waiting     chan struct{}
	waitingOnce sync.Once
}

func (stream *closedOnCancelStream) Read(buffer []byte) (int, error) {
	stream.mu.Lock()
	if len(stream.output) > 0 {
		count := copy(buffer, stream.output)
		stream.output = stream.output[count:]
		stream.mu.Unlock()
		return count, nil
	}
	stream.mu.Unlock()
	stream.waitingOnce.Do(func() { close(stream.waiting) })
	<-stream.closed
	return 0, errClosedConnection
}

func (*closedOnCancelStream) Write(buffer []byte) (int, error)   { return len(buffer), nil }
func (*closedOnCancelStream) SetReadTimeout(time.Duration) error { return nil }

// 呼び出し元が止めたために接続が閉じたなら、読み込みの失敗ではなく取り消しとして返す。
// SIGTERM で止めた Telnet の自動処理が 130 でなく 1 で終わるのを防ぐ。
func TestRunReportsCancellationWhenTheCallerClosesTheStreamFirst(t *testing.T) {
	cases := []struct {
		name   string
		output string
		settle time.Duration
	}{
		{name: "while waiting for the expected output"},
		{name: "while the matched output settles", output: "router# ", settle: 10 * time.Millisecond},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stopped, stop := context.WithCancel(context.Background())
			defer stop()
			caller := &heldChildCancellationContext{Context: stopped}
			defer caller.release()
			stream := &closedOnCancelStream{output: []byte(testCase.output), closed: stopped.Done(), waiting: make(chan struct{})}
			go func() {
				<-stream.waiting
				stop()
			}()

			// Run 全体の期限は、この試験の間に切れない長さにする。
			_, err := Run(caller, stream, Script{Steps: []Step{{Expect: "router# ", LineEnding: EndingNone}}}, Options{
				Timeout: time.Minute, Settle: testCase.settle,
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %#v, want context.Canceled", err)
			}
		})
	}
}
