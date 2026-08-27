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

	pending := make([]byte, 0, 32<<10)
	appendRead := func(data []byte) (overflow bool) {
		if len(result.Transcript)+len(data) > options.MaxBytes {
			remaining := options.MaxBytes - len(result.Transcript)
			if remaining > 0 {
				result.Transcript = append(result.Transcript, data[:remaining]...)
				pending = append(pending, data[:remaining]...)
			}
			return true
		}
		result.Transcript = append(result.Transcript, data...)
		pending = append(pending, data...)
		return false
	}

	for index, compiled := range steps {
		step := compiled.step
		stepCtx := runCtx
		cancelStep := func() {}
		if step.Timeout > 0 {
			stepCtx, cancelStep = context.WithTimeout(runCtx, step.Timeout)
		}
		defer cancelStep()
		if step.Send != nil || step.SendEnv != "" {
			value := ""
			if step.Send != nil {
				value = *step.Send
			} else {
				if options.LookupEnv == nil {
					return result, &Error{Kind: FailureEnvironment, Step: index, Err: errors.New("environment lookup is unavailable")}
				}
				var ok bool
				value, ok = options.LookupEnv(step.SendEnv)
				if !ok {
					return result, &Error{Kind: FailureEnvironment, Step: index, Err: fmt.Errorf("environment variable %q is not set", step.SendEnv)}
				}
				if value != "" {
					result.Secrets = append(result.Secrets, Secret{
						Value: []byte(value), TranscriptStart: len(result.Transcript),
					})
				}
			}
			ending, _ := step.LineEnding.bytes()
			payload := make([]byte, 0, len(value)+len(ending))
			payload = append(payload, value...)
			payload = append(payload, ending...)
			if len(payload) > MaxSendBytes+2 {
				return result, &Error{Kind: FailureInvalid, Step: index, Err: fmt.Errorf("send exceeds %d bytes", MaxSendBytes)}
			}
			if discarder, ok := stream.(interface{ DiscardPending(context.Context) error }); ok {
				if err := discarder.DiscardPending(stepCtx); err != nil {
					cancelStep()
					if stepCtx.Err() != nil {
						return result, timeoutOrCancellation(ctx, index, "stale input drain did not complete before timeout")
					}
					return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not discard stale stream input")}
				}
			}
			if err := writeWithin(stepCtx, stream, payload); err != nil {
				cancelStep()
				if stepCtx.Err() != nil {
					return result, timeoutOrCancellation(ctx, index, "stream write did not complete before timeout")
				}
				return result, &Error{Kind: FailureWrite, Step: index, Err: errors.New("could not write to the stream")}
			}
			cancelStep()
			// Bytes observed before this send cannot prove that the command
			// completed. A later expect must see a new response.
			pending = pending[:0]
			result.StepsCompleted = index + 1
			continue
		}

		if step.ReadFor != 0 {
			timer := time.NewTimer(step.ReadFor)
		readLoop:
			for {
				read := readOnce(stream)
				select {
				case <-stepCtx.Done():
					timer.Stop()
					cancelStep()
					return result, timeoutOrCancellation(ctx, index, "read interval did not complete before timeout")
				case <-timer.C:
					break readLoop
				case item := <-read:
					if len(item.data) == 0 && item.err == nil {
						timer.Stop()
						cancelStep()
						return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("stream made no read progress")}
					}
					if len(item.data) > 0 {
						if appendRead(item.data) {
							timer.Stop()
							cancelStep()
							return result, outputLimitError(index)
						}
					}
					if item.err != nil {
						timer.Stop()
						cancelStep()
						if stepCtx.Err() != nil {
							return result, timeoutOrCancellation(ctx, index, "read interval did not complete before timeout")
						}
						return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not read from the stream")}
					}
				}
			}
			cancelStep()
			result.StepsCompleted = index + 1
			continue
		}

		limitReached := false
		for {
			if location := compiled.expect.FindIndex(pending); location != nil {
				if !limitReached && options.Settle > 0 {
					item, supported, settleErr := readUntilQuiet(stepCtx, stream, options.Settle)
					if settleErr != nil {
						cancelStep()
						if stepCtx.Err() != nil {
							return result, timeoutOrCancellation(ctx, index, "expected output did not settle before timeout")
						}
						return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not verify that expected output settled")}
					}
					if supported && len(item.data) > 0 {
						limitReached = appendRead(item.data)
						if item.err != nil {
							cancelStep()
							return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not read from the stream")}
						}
						continue
					}
					if supported && item.err != nil {
						cancelStep()
						return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not read from the stream")}
					}
				}
				pending = append(pending[:0], pending[location[1]:]...)
				result.Matched = true
				result.LastExpectation = step.Expect
				result.StepsCompleted = index + 1
				cancelStep()
				break
			}
			if limitReached {
				cancelStep()
				return result, outputLimitError(index)
			}
			read := readOnce(stream)
			select {
			case <-stepCtx.Done():
				cancelStep()
				return result, timeoutOrCancellation(ctx, index, "expected output was not received before timeout")
			case item := <-read:
				if len(item.data) == 0 && item.err == nil {
					cancelStep()
					return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("stream made no read progress")}
				}
				if len(item.data) > 0 {
					limitReached = appendRead(item.data)
				}
				if item.err != nil {
					cancelStep()
					if stepCtx.Err() != nil {
						return result, timeoutOrCancellation(ctx, index, "expected output was not received before timeout")
					}
					if errors.Is(item.err, io.EOF) {
						return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("stream closed before expected output")}
					}
					return result, &Error{Kind: FailureRead, Step: index, Err: errors.New("could not read from the stream")}
				}
			}
		}
	}
	return result, nil
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
		buffer := make([]byte, 32<<10)
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

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		count, err := writer.Write(payload)
		if count < 0 || count > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[count:]
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
	}
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func writeWithin(ctx context.Context, writer io.Writer, payload []byte) error {
	written := make(chan error, 1)
	go func() { written <- writeAll(writer, payload) }()
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
