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

type transportScriptDocument struct {
	Version int                   `json:"version"`
	Steps   []transportScriptStep `json:"steps"`
}

type transportScriptStep struct {
	Expect     string  `json:"expect,omitempty"`
	Send       *string `json:"send,omitempty"`
	SendEnv    string  `json:"sendEnv,omitempty"`
	ReadFor    string  `json:"readFor,omitempty"`
	LineEnding string  `json:"lineEnding,omitempty"`
	Timeout    string  `json:"timeout,omitempty"`
}

func buildTransportScript(called transportInvocation, stdin io.Reader) (streamrun.Script, error) {
	if called.Script == "" {
		command := strings.Join(called.Command, " ")
		steps := []streamrun.Step{{Send: &command, LineEnding: called.LineEnding}}
		if called.Expect != "" {
			steps = append(steps, streamrun.Step{Expect: called.Expect, LineEnding: streamrun.EndingNone})
		} else {
			steps = append(steps, streamrun.Step{ReadFor: called.ReadFor, LineEnding: streamrun.EndingNone})
		}
		return streamrun.Script{Steps: steps}, nil
	}

	reader := stdin
	var file *os.File
	if called.Script != "-" {
		opened, err := os.Open(called.Script)
		if err != nil {
			return streamrun.Script{}, errors.New("could not open the script")
		}
		file = opened
		defer file.Close()
		reader = file
	}
	limited := io.LimitReader(reader, maxScriptBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return streamrun.Script{}, errors.New("could not read the script")
	}
	if len(payload) > maxScriptBytes {
		return streamrun.Script{}, fmt.Errorf("script exceeds %d bytes", maxScriptBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var document transportScriptDocument
	if err := decoder.Decode(&document); err != nil {
		return streamrun.Script{}, errors.New("script is not valid JSON")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return streamrun.Script{}, errors.New("script must contain one JSON document")
	}
	if document.Version != 1 {
		return streamrun.Script{}, errors.New("script version must be 1")
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
				return streamrun.Script{}, fmt.Errorf("script step %d readFor is not a duration", index+1)
			}
		}
		if source.Timeout != "" {
			step.Timeout, err = time.ParseDuration(source.Timeout)
			if err != nil || step.Timeout <= 0 {
				return streamrun.Script{}, fmt.Errorf("script step %d timeout is not a duration", index+1)
			}
		}
		steps[index] = step
	}
	return streamrun.Script{Steps: steps}, nil
}

type transportRunFailure struct {
	Kind    string `json:"kind"`
	Step    int    `json:"step,omitempty"`
	Message string `json:"message"`
}

type transportRunReport struct {
	SchemaVersion      int                  `json:"schemaVersion"`
	Transport          transportKind        `json:"transport"`
	Target             string               `json:"target"`
	Success            bool                 `json:"success"`
	Matched            bool                 `json:"matched"`
	Expectation        string               `json:"expectation,omitempty"`
	StepsCompleted     int                  `json:"stepsCompleted"`
	Transcript         string               `json:"transcript,omitempty"`
	TranscriptBase64   string               `json:"transcriptBase64,omitempty"`
	TranscriptEncoding string               `json:"transcriptEncoding"`
	DurationMillis     int64                `json:"durationMillis"`
	Warnings           []string             `json:"warnings,omitempty"`
	Failure            *transportRunFailure `json:"failure,omitempty"`
}

func newTransportRunReport(called transportInvocation, result streamrun.Result, runErr error, warnings []string) transportRunReport {
	transcript := streamrun.Redact(result.Transcript, result.Secrets)
	report := transportRunReport{
		SchemaVersion: 1, Transport: called.Transport, Target: called.Target,
		Success: runErr == nil, Matched: result.Matched, Expectation: result.LastExpectation,
		StepsCompleted: result.StepsCompleted, DurationMillis: result.Duration.Milliseconds(),
		Warnings: warnings,
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
