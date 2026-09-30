// Package streamrun executes bounded expect/send conversations over byte streams.
package streamrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"sshc/internal/iowrite"
)

const (
	MaxSteps        = 128
	MaxPatternBytes = 4096
	MaxSendBytes    = 64 << 10
	DefaultMaxBytes = 1 << 20
	MaxMaxBytes     = 16 << 20
	DefaultTimeout  = 10 * time.Second
	MaxTimeout      = 5 * time.Minute
	MaxSettle       = 5 * time.Second
)

type FailureKind string

const (
	FailureInvalid     FailureKind = "invalid_script"
	FailureTimeout     FailureKind = "timeout"
	FailureOutputLimit FailureKind = "output_limit"
	FailureRead        FailureKind = "read_failed"
	FailureWrite       FailureKind = "write_failed"
	FailureEnvironment FailureKind = "environment_missing"
)

// Error exposes a stable failure kind without leaking stream implementation details.
type Error struct {
	Kind FailureKind
	Step int
	Err  error
}

func (e *Error) Error() string {
	if e.Step >= 0 {
		return fmt.Sprintf("stream step %d: %v", e.Step+1, e.Err)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

type LineEnding string

const (
	EndingNone LineEnding = "none"
	EndingCR   LineEnding = "cr"
	EndingLF   LineEnding = "lf"
	EndingCRLF LineEnding = "crlf"
)

func (ending LineEnding) bytes() ([]byte, bool) {
	switch ending {
	case EndingNone:
		return nil, true
	case EndingCR:
		return []byte{'\r'}, true
	case EndingLF:
		return []byte{'\n'}, true
	case EndingCRLF:
		return []byte{'\r', '\n'}, true
	default:
		return nil, false
	}
}

// Step performs exactly one action. Send is a pointer so an empty line remains
// distinguishable from an omitted send action.
type Step struct {
	Expect     string
	Send       *string
	SendEnv    string
	ReadFor    time.Duration
	LineEnding LineEnding
	Timeout    time.Duration
}

type Script struct {
	Steps []Step
}

type Options struct {
	Timeout   time.Duration
	MaxBytes  int
	LookupEnv func(string) (string, bool)
	Settle    time.Duration
}

type Result struct {
	Transcript      []byte
	Matched         bool
	LastExpectation string
	StepsCompleted  int
	Duration        time.Duration
	Secrets         []Secret
}

// Secret records where output produced after a sendEnv may begin. Redaction
// uses the boundary to mask a truncated echo without corrupting earlier text.
type Secret struct {
	Value           []byte
	TranscriptStart int
}

type compiledStep struct {
	step   Step
	expect *regexp.Regexp
}

func validate(script Script, options *Options) ([]compiledStep, error) {
	if len(script.Steps) == 0 || len(script.Steps) > MaxSteps {
		return nil, &Error{Kind: FailureInvalid, Step: -1, Err: fmt.Errorf("script must contain between 1 and %d steps", MaxSteps)}
	}
	if options.Timeout == 0 {
		options.Timeout = DefaultTimeout
	}
	if options.Timeout < 0 || options.Timeout > MaxTimeout {
		return nil, &Error{Kind: FailureInvalid, Step: -1, Err: fmt.Errorf("timeout must be between 1ns and %s", MaxTimeout)}
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	if options.MaxBytes < 1 || options.MaxBytes > MaxMaxBytes {
		return nil, &Error{Kind: FailureInvalid, Step: -1, Err: fmt.Errorf("max bytes must be between 1 and %d", MaxMaxBytes)}
	}
	if options.Settle < 0 || options.Settle > MaxSettle {
		return nil, &Error{Kind: FailureInvalid, Step: -1, Err: fmt.Errorf("settle must be between zero and %s", MaxSettle)}
	}

	compiled := make([]compiledStep, len(script.Steps))
	for index, step := range script.Steps {
		actions := 0
		if step.Expect != "" {
			actions++
		}
		if step.Send != nil {
			actions++
		}
		if step.SendEnv != "" {
			actions++
		}
		if step.ReadFor != 0 {
			actions++
		}
		if actions != 1 {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: errors.New("a step must contain exactly one of expect, send, sendEnv, or readFor")}
		}
		if step.Timeout < 0 || step.Timeout > MaxTimeout {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("step timeout must not exceed %s", MaxTimeout)}
		}
		if _, ok := step.LineEnding.bytes(); !ok {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: errors.New("line ending must be none, cr, lf, or crlf")}
		}
		if step.Expect != "" {
			if len(step.Expect) > MaxPatternBytes {
				return nil, &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("expect pattern exceeds %d bytes", MaxPatternBytes)}
			}
			pattern, err := regexp.Compile(step.Expect)
			if err != nil {
				return nil, &Error{Kind: FailureInvalid, Step: index, Err: errors.New("expect pattern is not valid")}
			}
			if pattern.Match(nil) {
				return nil, &Error{Kind: FailureInvalid, Step: index, Err: errors.New("expect pattern must not match empty input")}
			}
			compiled[index].expect = pattern
		}
		if step.Send != nil && len(*step.Send) > MaxSendBytes {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("send exceeds %d bytes", MaxSendBytes)}
		}
		if step.ReadFor < 0 || step.ReadFor > MaxTimeout {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("readFor must not exceed %s", MaxTimeout)}
		}
		if step.ReadFor != 0 && index != len(script.Steps)-1 {
			return nil, &Error{Kind: FailureInvalid, Step: index, Err: errors.New("readFor must be the final script step")}
		}
		compiled[index].step = step
	}
	return compiled, nil
}

// Validate checks the complete script and resource limits without touching the
// stream. Callers can use it before opening a device or network connection.
func Validate(script Script, options Options) error {
	_, err := validate(script, &options)
	return err
}

type readResult struct {
	data []byte
	err  error
}

// Run executes a script without allowing unbounded input, regular-expression
// backtracking, or a blocked Read/Write to defeat a timeout. Timeout is the
// deadline for the complete script; a step Timeout may shorten that deadline.
// The caller must close the stream after Run returns so a blocked I/O goroutine
// can exit.
func Run(ctx context.Context, stream io.ReadWriter, script Script, options Options) (result Result, runErr error) {
	started := time.Now()
	defer func() { result.Duration = time.Since(started) }()
	steps, err := validate(script, &options)
	if err != nil {
		return result, err
	}
	runCtx, cancelRun := context.WithTimeout(ctx, options.Timeout)
	defer cancelRun()

	run := &scriptRun{
		caller: ctx, stream: stream, options: options, result: &result,
		pending: make([]byte, 0, readChunkBytes),
	}
	for index, compiled := range steps {
		if err := run.runStep(runCtx, index, compiled); err != nil {
			return result, err
		}
		result.StepsCompleted = index + 1
	}
	return result, nil
}

// readChunkBytes は、1回の Read で受け取る大きさ。io.Copy の既定と同じ 32 KiB にする。
// pending もこの大きさから始め、最初の読み取りで伸ばさずに済むようにする。
const readChunkBytes = 32 << 10

// staleInputQuiet は、送信の前に古い入力を読み捨てるとき、入力が止んだとみなす無音の長さ。
// 9600bps でも約19文字を送れる長さなので、バナーを送っている途中の文字の間とは区別できる。
// 送信のたびに足される待ちでもあるので、短く保つ。
const staleInputQuiet = 20 * time.Millisecond

// staleInputDiscarder は、送信の前に古いバナーやプロンプトを読み捨てられる stream。
// 読み捨てないと、送信前から届いていたプロンプトをコマンドの応答と取り違える。
type staleInputDiscarder interface {
	DiscardPending(ctx context.Context, quiet time.Duration) error
}

const (
	readForTimeoutMessage = "read interval did not complete before timeout"
	expectTimeoutMessage  = "expected output was not received before timeout"
)

// scriptRun は、1回の Run のあいだ段をまたいで持ち越す状態をまとめる。
type scriptRun struct {
	// caller は Run の呼び出し元の context。期限切れと呼び出し元による停止を分けるのに使う。
	caller  context.Context
	stream  io.ReadWriter
	options Options
	result  *Result
	// pending は、直前の send より後に届き、まだどの expect の一致にも使っていない出力。
	pending []byte
}

// runStep は1つの段を実行する。段の期限はこの関数の中だけで生きるので、失敗の分類は
// どれも期限を解放する前に終わる。
func (run *scriptRun) runStep(runCtx context.Context, index int, compiled compiledStep) error {
	stepCtx := runCtx
	if compiled.step.Timeout > 0 {
		var cancelStep context.CancelFunc
		stepCtx, cancelStep = context.WithTimeout(runCtx, compiled.step.Timeout)
		defer cancelStep()
	}
	switch {
	case compiled.step.Send != nil || compiled.step.SendEnv != "":
		return run.send(stepCtx, index, compiled.step)
	case compiled.step.ReadFor != 0:
		return run.readFor(stepCtx, index, compiled.step.ReadFor)
	default:
		return run.expect(stepCtx, index, compiled)
	}
}

func (run *scriptRun) send(stepCtx context.Context, index int, step Step) error {
	value, err := sendValue(index, step, run.options.LookupEnv)
	if err != nil {
		return err
	}
	if step.SendEnv != "" && value != "" {
		run.result.Secrets = append(run.result.Secrets, Secret{
			Value: []byte(value), TranscriptStart: len(run.result.Transcript),
		})
	}
	if len(value) > MaxSendBytes {
		return &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("send exceeds %d bytes", MaxSendBytes)}
	}
	ending, _ := step.LineEnding.bytes()
	payload := append([]byte(value), ending...)
	if discarder, ok := run.stream.(staleInputDiscarder); ok {
		if err := discarder.DiscardPending(stepCtx, staleInputQuiet); err != nil {
			failure := &Error{Kind: FailureRead, Step: index, Err: errors.New("could not discard stale stream input")}
			return run.stepFailure(stepCtx, failure, "stale input drain did not complete before timeout")
		}
	}
	if err := writeWithin(stepCtx, run.stream, payload); err != nil {
		failure := &Error{Kind: FailureWrite, Step: index, Err: errors.New("could not write to the stream")}
		return run.stepFailure(stepCtx, failure, "stream write did not complete before timeout")
	}
	// Bytes observed before this send cannot prove that the command
	// completed. A later expect must see a new response.
	run.pending = run.pending[:0]
	return nil
}

// sendValue は、段が送る文字列を返す。sendEnv なら環境変数から引く。
func sendValue(index int, step Step, lookupEnv func(string) (string, bool)) (string, error) {
	if step.Send != nil {
		return *step.Send, nil
	}
	if lookupEnv == nil {
		return "", &Error{Kind: FailureEnvironment, Step: index, Err: errors.New("environment lookup is unavailable")}
	}
	value, ok := lookupEnv(step.SendEnv)
	if !ok {
		return "", &Error{Kind: FailureEnvironment, Step: index, Err: fmt.Errorf("environment variable %q is not set", step.SendEnv)}
	}
	return value, nil
}

func (run *scriptRun) readFor(stepCtx context.Context, index int, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		read := readOnce(run.stream)
		select {
		case <-stepCtx.Done():
			return timeoutOrCancellation(run.caller, index, readForTimeoutMessage)
		case <-timer.C:
			return nil
		case item := <-read:
			if err := run.keepIntervalOutput(stepCtx, index, item); err != nil {
				return err
			}
		}
	}
}

// keepIntervalOutput は、readFor の読み取り1回分を transcript に足す。readFor は時間で
// 終わる段なので、上限を超えた出力も、時間より前の EOF も失敗にする。
func (run *scriptRun) keepIntervalOutput(stepCtx context.Context, index int, item readResult) error {
	if len(item.data) == 0 && item.err == nil {
		return noReadProgressError(index)
	}
	if len(item.data) > 0 && run.appendRead(item.data) {
		return outputLimitError(index)
	}
	if item.err != nil {
		return run.stepFailure(stepCtx, readFailedError(index), readForTimeoutMessage)
	}
	return nil
}

func (run *scriptRun) expect(stepCtx context.Context, index int, compiled compiledStep) error {
	limitReached := false
	for {
		location := compiled.expect.FindIndex(run.pending)
		if location == nil {
			if limitReached {
				return outputLimitError(index)
			}
			overflow, err := run.readExpected(stepCtx, index)
			if err != nil {
				return err
			}
			limitReached = overflow
			continue
		}
		if !limitReached && run.options.Settle > 0 {
			arrived, overflow, err := run.readWhileSettling(stepCtx, index)
			if err != nil {
				return err
			}
			if arrived {
				limitReached = overflow
				continue
			}
		}
		run.pending = append(run.pending[:0], run.pending[location[1]:]...)
		run.result.Matched = true
		run.result.LastExpectation = compiled.step.Expect
		return nil
	}
}

// readExpected は、expect の一致を探すために1回読み、出力が上限を超えたかを返す。
func (run *scriptRun) readExpected(stepCtx context.Context, index int) (overflow bool, err error) {
	read := readOnce(run.stream)
	select {
	case <-stepCtx.Done():
		return false, timeoutOrCancellation(run.caller, index, expectTimeoutMessage)
	case item := <-read:
		if len(item.data) == 0 && item.err == nil {
			return false, noReadProgressError(index)
		}
		if len(item.data) > 0 {
			overflow = run.appendRead(item.data)
		}
		if item.err == nil {
			return overflow, nil
		}
		failure := readFailedError(index)
		if errors.Is(item.err, io.EOF) {
			failure.Err = errors.New("stream closed before expected output")
		}
		return false, run.stepFailure(stepCtx, failure, expectTimeoutMessage)
	}
}

// readWhileSettling は、一致した出力のあとに続きが届かないかを Settle のあいだ待つ。
// 続きが届いたら arrived を返し、呼び出し側は続きを含めて一致を探し直す。
func (run *scriptRun) readWhileSettling(stepCtx context.Context, index int) (arrived, overflow bool, err error) {
	item, supported, settleErr := readUntilQuiet(stepCtx, run.stream, run.options.Settle)
	if settleErr != nil {
		failure := &Error{Kind: FailureRead, Step: index, Err: errors.New("could not verify that expected output settled")}
		return false, false, run.stepFailure(stepCtx, failure, "expected output did not settle before timeout")
	}
	if !supported {
		return false, false, nil
	}
	if len(item.data) > 0 {
		overflow = run.appendRead(item.data)
	}
	if item.err != nil {
		return false, false, readFailedError(index)
	}
	return len(item.data) > 0, overflow, nil
}

// appendRead は読んだ出力を transcript と pending に足す。MaxBytes を超える分は捨てて true を返す。
func (run *scriptRun) appendRead(data []byte) (overflow bool) {
	remaining := run.options.MaxBytes - len(run.result.Transcript)
	if len(data) > remaining {
		data = data[:max(remaining, 0)]
		overflow = true
	}
	run.result.Transcript = append(run.result.Transcript, data...)
	run.pending = append(run.pending, data...)
	return overflow
}

// stepFailure は、段の入出力の失敗を返す。段の期限か Run 全体の期限が先に切れていたら、
// 入出力が失敗した原因はそちらなので timeout（呼び出し元が止めたならその err）を返す。
// 段の context を解放すると Err が必ず Canceled になるので、解放より前に呼ぶ。
func (run *scriptRun) stepFailure(stepCtx context.Context, failure *Error, timeoutMessage string) error {
	if stepCtx.Err() != nil {
		return timeoutOrCancellation(run.caller, failure.Step, timeoutMessage)
	}
	return failure
}

type streamReadTimeoutSetter interface {
	// SetReadTimeout bounds a following Read. Zero restores blocking reads.
	SetReadTimeout(time.Duration) error
}

func readUntilQuiet(ctx context.Context, reader io.Reader, quiet time.Duration) (readResult, bool, error) {
	setter, ok := reader.(streamReadTimeoutSetter)
	if !ok {
		return readResult{}, false, nil
	}
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return readResult{}, true, ctx.Err()
		}
		if remaining < quiet {
			quiet = remaining
		}
	}
	if err := setter.SetReadTimeout(quiet); err != nil {
		return readResult{}, true, err
	}
	read := readOnce(reader)
	select {
	case <-ctx.Done():
		_ = setter.SetReadTimeout(0)
		return readResult{}, true, ctx.Err()
	case item := <-read:
		if err := setter.SetReadTimeout(0); err != nil {
			return readResult{}, true, err
		}
		if ctx.Err() != nil {
			return readResult{}, true, ctx.Err()
		}
		return item, true, nil
	}
}

func readOnce(reader io.Reader) <-chan readResult {
	completed := make(chan readResult, 1)
	go func() {
		buffer := make([]byte, readChunkBytes)
		count, err := reader.Read(buffer)
		item := readResult{err: err}
		if count > 0 {
			item.data = append([]byte(nil), buffer[:count]...)
		}
		completed <- item
	}()
	return completed
}

func timeoutOrCancellation(parent context.Context, step int, message string) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	return &Error{Kind: FailureTimeout, Step: step, Err: errors.New(message)}
}

func outputLimitError(step int) error {
	return &Error{Kind: FailureOutputLimit, Step: step, Err: errors.New("stream output exceeded the configured limit")}
}

func readFailedError(step int) *Error {
	return &Error{Kind: FailureRead, Step: step, Err: errors.New("could not read from the stream")}
}

func noReadProgressError(step int) *Error {
	return &Error{Kind: FailureRead, Step: step, Err: errors.New("stream made no read progress")}
}

// WriteAll は payload を iowrite.WriteAll で残らず書き、書き手が Flush を持てば最後に
// flush する。対話 transport の attach も同じ規則で書く。
func WriteAll(writer io.Writer, payload []byte) error {
	if err := iowrite.WriteAll(writer, payload); err != nil {
		return err
	}
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func writeWithin(ctx context.Context, writer io.Writer, payload []byte) error {
	written := make(chan error, 1)
	go func() { written <- WriteAll(writer, payload) }()
	select {
	case err := <-written:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Redact replaces exact secret byte sequences before a transcript leaves the
// process. It intentionally operates on bytes so invalid UTF-8 cannot bypass it.
func Redact(transcript []byte, secrets []Secret) []byte {
	redacted := append([]byte(nil), transcript...)
	for index := len(secrets) - 1; index >= 0; index-- {
		secret := secrets[index]
		if len(secret.Value) == 0 {
			continue
		}
		start := min(max(secret.TranscriptStart, 0), len(redacted))
		segment := redacted[start:]
		segment = bytes.ReplaceAll(segment, secret.Value, []byte("[REDACTED]"))
		// Password echoes are line-oriented. Before the first newline after
		// sendEnv, mask a secret prefix only when it reaches the captured
		// boundary. Searching arbitrary prefixes would corrupt ordinary output
		// such as "shell ready" when the secret also begins with "s".
		lineEnd := len(segment)
		if offset := bytes.IndexAny(segment, "\r\n"); offset >= 0 {
			lineEnd = offset
		}
		line := redactTruncatedSecretSuffix(segment[:lineEnd], secret.Value)
		combined := make([]byte, 0, len(line)+len(segment)-lineEnd)
		combined = append(combined, line...)
		combined = append(combined, segment[lineEnd:]...)
		redacted = append(redacted[:start], combined...)
	}
	return redacted
}

func redactTruncatedSecretSuffix(line, secret []byte) []byte {
	for length := min(len(secret)-1, len(line)); length > 0; length-- {
		start := len(line) - length
		if start > 0 && isSecretWordByte(line[start-1]) {
			continue
		}
		if bytes.Equal(line[start:], secret[:length]) {
			redacted := append([]byte(nil), line[:start]...)
			return append(redacted, []byte("[REDACTED]")...)
		}
	}
	return append([]byte(nil), line...)
}

func isSecretWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_'
}
