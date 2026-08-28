package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"sshc/internal/streamrun"
)

const maxScriptBytes = 1 << 20

const (
	defaultFailureCleanupTimeout = 500 * time.Millisecond
	maxFailureCleanupTimeout     = 5 * time.Second
)

type transportScriptDocument struct {
	Version   int                              `json:"version"`
	Steps     []transportScriptStep            `json:"steps"`
	OnFailure *transportFailureCleanupDocument `json:"onFailure,omitempty"`
}

type transportScriptStep struct {
	Expect     string  `json:"expect,omitempty"`
	Send       *string `json:"send,omitempty"`
	SendEnv    string  `json:"sendEnv,omitempty"`
	ReadFor    string  `json:"readFor,omitempty"`
	LineEnding string  `json:"lineEnding,omitempty"`
	Timeout    string  `json:"timeout,omitempty"`
}

type transportFailureCleanupDocument struct {
	Send       *string `json:"send"`
	LineEnding string  `json:"lineEnding,omitempty"`
	Timeout    string  `json:"timeout,omitempty"`
}

type transportFailureCleanup struct {
	Send       string
	LineEnding streamrun.LineEnding
	Timeout    time.Duration
}

type builtTransportScript struct {
	streamrun.Script
	OnFailure *transportFailureCleanup
}

func buildTransportScript(called transportInvocation, stdin io.Reader) (builtTransportScript, error) {
	if called.Script == "" {
		command := strings.Join(called.Command, " ")
		steps := []streamrun.Step{{Send: &command, LineEnding: called.LineEnding}}
		if called.Expect != "" {
			steps = append(steps, streamrun.Step{Expect: called.Expect, LineEnding: streamrun.EndingNone})
		} else {
			steps = append(steps, streamrun.Step{ReadFor: called.ReadFor, LineEnding: streamrun.EndingNone})
		}
		return builtTransportScript{Script: streamrun.Script{Steps: steps}}, nil
	}

	reader := stdin
	var file *os.File
	if called.Script != "-" {
		opened, err := os.Open(called.Script)
		if err != nil {
			return builtTransportScript{}, errors.New("could not open the script")
		}
		file = opened
		defer file.Close()
		reader = file
	}
	limited := io.LimitReader(reader, maxScriptBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return builtTransportScript{}, errors.New("could not read the script")
	}
	if len(payload) > maxScriptBytes {
		return builtTransportScript{}, fmt.Errorf("script exceeds %d bytes", maxScriptBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var document transportScriptDocument
	if err := decoder.Decode(&document); err != nil {
		return builtTransportScript{}, errors.New("script is not valid JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return builtTransportScript{}, errors.New("script must contain one JSON document")
	}
	if document.Version != 1 {
		return builtTransportScript{}, errors.New("script version must be 1")
	}

	steps := make([]streamrun.Step, len(document.Steps))
	for index, source := range document.Steps {
		ending := called.LineEnding
		if source.LineEnding != "" {
			ending = streamrun.LineEnding(source.LineEnding)
		}
		step := streamrun.Step{Expect: source.Expect, Send: source.Send, SendEnv: source.SendEnv, LineEnding: ending}
		if source.Expect != "" || source.ReadFor != "" {
			step.LineEnding = streamrun.EndingNone
		}
		if source.ReadFor != "" {
			step.ReadFor, err = time.ParseDuration(source.ReadFor)
			if err != nil {
				return builtTransportScript{}, fmt.Errorf("script step %d readFor is not a duration", index+1)
			}
		}
		if source.Timeout != "" {
			step.Timeout, err = time.ParseDuration(source.Timeout)
			if err != nil || step.Timeout <= 0 {
				return builtTransportScript{}, fmt.Errorf("script step %d timeout is not a duration", index+1)
			}
		}
		steps[index] = step
	}
	built := builtTransportScript{Script: streamrun.Script{Steps: steps}}
	if document.OnFailure != nil {
		cleanup, err := buildTransportFailureCleanup(*document.OnFailure)
		if err != nil {
			return builtTransportScript{}, err
		}
		built.OnFailure = &cleanup
	}
	return built, nil
}

func buildTransportFailureCleanup(document transportFailureCleanupDocument) (transportFailureCleanup, error) {
	if document.Send == nil {
		return transportFailureCleanup{}, errors.New("script onFailure.send is required")
	}
	cleanup := transportFailureCleanup{
		Send: *document.Send, LineEnding: streamrun.EndingNone, Timeout: defaultFailureCleanupTimeout,
	}
	if document.LineEnding != "" {
		cleanup.LineEnding = streamrun.LineEnding(document.LineEnding)
	}
	ending, ok := transportLineEndingBytes(cleanup.LineEnding)
	if !ok {
		return transportFailureCleanup{}, errors.New("script onFailure.lineEnding must be none, cr, lf, or crlf")
	}
	if len(cleanup.Send) > streamrun.MaxSendBytes || len(cleanup.Send)+len(ending) > streamrun.MaxSendBytes+2 {
		return transportFailureCleanup{}, fmt.Errorf("script onFailure.send exceeds %d bytes", streamrun.MaxSendBytes)
	}
	if document.Timeout != "" {
		var err error
		cleanup.Timeout, err = time.ParseDuration(document.Timeout)
		if err != nil || cleanup.Timeout <= 0 || cleanup.Timeout > maxFailureCleanupTimeout {
			return transportFailureCleanup{}, fmt.Errorf("script onFailure.timeout must be between 1ns and %s", maxFailureCleanupTimeout)
		}
	}
	return cleanup, nil
}

func transportLineEndingBytes(ending streamrun.LineEnding) ([]byte, bool) {
	switch ending {
	case streamrun.EndingNone:
		return nil, true
	case streamrun.EndingCR:
		return []byte{'\r'}, true
	case streamrun.EndingLF:
		return []byte{'\n'}, true
	case streamrun.EndingCRLF:
		return []byte{'\r', '\n'}, true
	default:
		return nil, false
	}
}

type transportRunFailure struct {
	Kind    string `json:"kind"`
	Step    int    `json:"step,omitempty"`
	Message string `json:"message"`
}

type transportRunReport struct {
	SchemaVersion      int                            `json:"schemaVersion"`
	Transport          transportKind                  `json:"transport"`
	Target             string                         `json:"target"`
	Success            bool                           `json:"success"`
	Matched            bool                           `json:"matched"`
	Expectation        string                         `json:"expectation,omitempty"`
	StepsCompleted     int                            `json:"stepsCompleted"`
	Transcript         string                         `json:"transcript,omitempty"`
	TranscriptBase64   string                         `json:"transcriptBase64,omitempty"`
	TranscriptEncoding string                         `json:"transcriptEncoding"`
	DurationMillis     int64                          `json:"durationMillis"`
	BytesReceived      int                            `json:"bytesReceived"`
	Warnings           []string                       `json:"warnings,omitempty"`
	Failure            *transportRunFailure           `json:"failure,omitempty"`
	FailureCleanup     *transportFailureCleanupReport `json:"failureCleanup,omitempty"`
}

type transportFailureCleanupReport struct {
	Attempted bool   `json:"attempted"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func newTransportRunReport(called transportInvocation, result streamrun.Result, runErr error, warnings []string, cleanup *transportFailureCleanupReport) transportRunReport {
	transcript := streamrun.Redact(result.Transcript, result.Secrets)
	report := transportRunReport{
		SchemaVersion: 1, Transport: called.Transport, Target: called.Target,
		Success: runErr == nil, Matched: result.Matched, Expectation: result.LastExpectation,
		StepsCompleted: result.StepsCompleted, DurationMillis: result.Duration.Milliseconds(),
		BytesReceived: len(result.Transcript), Warnings: warnings, FailureCleanup: cleanup,
	}
	if utf8.Valid(transcript) {
		report.TranscriptEncoding = "utf-8"
		report.Transcript = string(transcript)
	} else {
		report.TranscriptEncoding = "base64"
		report.TranscriptBase64 = base64.StdEncoding.EncodeToString(transcript)
	}
	if runErr != nil {
		report.Failure = &transportRunFailure{Kind: "stream_failed", Message: runErr.Error()}
		if errors.Is(runErr, errNoTransportOutput) {
			report.Failure.Kind = "no_output"
		}
		var failure *streamrun.Error
		if errors.As(runErr, &failure) {
			report.Failure.Kind = string(failure.Kind)
			report.Failure.Step = failure.Step + 1
		}
		if errors.Is(runErr, context.Canceled) {
			report.Failure.Kind = "interrupted"
			report.Failure.Message = "operation was interrupted"
		}
	}
	return report
}

func writeTransportReport(out io.Writer, report transportRunReport) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}
