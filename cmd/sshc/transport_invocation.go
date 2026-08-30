package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"sshc/internal/streamrun"
	"sshc/internal/textencoding"
)

type transportKind string

const (
	transportSerial transportKind = "serial"
	transportTelnet transportKind = "telnet"
)

type transportInvocation struct {
	Transport      transportKind
	Target         string
	List           bool
	JSON           bool
	Run            bool
	RequireOutput  bool
	Command        []string
	Script         string
	Expect         string
	ReadFor        time.Duration
	Timeout        time.Duration
	ConnectTimeout time.Duration
	MaxBytes       int
	LineEnding     streamrun.LineEnding
	Settle         time.Duration

	Baud         int
	DataBits     int
	Parity       string
	StopBits     string
	Flow         string
	DTR          *bool
	RTS          *bool
	Break        time.Duration
	TerminalType string
	Encoding     textencoding.Name
}

func defaultTransportInvocation(kind transportKind, run bool) transportInvocation {
	called := transportInvocation{
		Transport:      kind,
		Run:            run,
		Timeout:        streamrun.DefaultTimeout,
		ConnectTimeout: 10 * time.Second,
		MaxBytes:       streamrun.DefaultMaxBytes,
		Baud:           9600,
		DataBits:       8,
		Parity:         "none",
		StopBits:       "1",
		Flow:           "none",
		TerminalType:   "xterm-256color",
		Encoding:       textencoding.UTF8,
		Settle:         120 * time.Millisecond,
	}
	if kind == transportSerial {
		called.LineEnding = streamrun.EndingCR
	} else {
		called.LineEnding = streamrun.EndingCRLF
	}
	return called
}

func parseTransportInvocation(kind transportKind, args []string) (invocation, error) {
	if helpRequested(args) {
		return helpInvocation(string(kind)), nil
	}
	called := defaultTransportInvocation(kind, false)
	if kind == transportSerial && (len(args) == 0 || len(args) == 1 && args[0] == "--json") {
		called.List = true
		called.JSON = len(args) == 1
		return invocation{Kind: invocationTransport, Transport: &called}, nil
	}
	if kind == transportSerial && len(args) > 0 && args[0] == "list" {
		return invalidInvocation("serial no longer takes list; use sshc serial or sshc serial --json")
	}

	delimited := false
	seenOptions := make(map[string]struct{})
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if called.Run && argument == "--" {
			delimited = true
			called.Command = copyInvocationArgs(args[index+1:])
			break
		}
		if strings.HasPrefix(argument, "--") {
			name, inline, hasInline := strings.Cut(argument, "=")
			if _, duplicate := seenOptions[name]; duplicate {
				return invalidInvocation("transport option " + name + " was specified more than once")
			}
			seenOptions[name] = struct{}{}
			takeValue := func() (string, error) {
				if hasInline {
					return inline, nil
				}
				index++
				if index >= len(args) {
					return "", fmt.Errorf("%s takes a value", name)
				}
				return args[index], nil
			}
			switch name {
			case "--non-interactive":
				if hasInline {
					return invalidInvocation("--non-interactive does not take a value")
				}
				called.Run = true
			case "--require-output":
				if hasInline {
					return invalidInvocation("--require-output does not take a value")
				}
				if !called.Run {
					return invalidInvocation("--require-output requires --non-interactive")
				}
				called.RequireOutput = true
			case "--json":
				if hasInline {
					return invalidInvocation("--json does not take a value")
				}
				called.JSON = true
			case "--encoding":
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Encoding, err = textencoding.Parse(value)
				if err != nil {
					return invalidInvocation("--encoding takes utf-8, shift_jis, euc-jp, or iso-2022-jp")
				}
			case "--baud":
				if kind != transportSerial {
					return invalidInvocation("--baud can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Baud, err = positiveInt(name, value)
				if err != nil || called.Baud > 4_000_000 {
					return invalidInvocation("--baud takes a number between 1 and 4000000")
				}
			case "--data-bits":
				if kind != transportSerial {
					return invalidInvocation("--data-bits can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.DataBits, err = strconv.Atoi(value)
				if err != nil || called.DataBits < 5 || called.DataBits > 8 {
					return invalidInvocation("--data-bits takes 5, 6, 7, or 8")
				}
			case "--parity":
				if kind != transportSerial {
					return invalidInvocation("--parity can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if value != "none" && value != "odd" && value != "even" && value != "mark" && value != "space" {
					return invalidInvocation("--parity takes none, odd, even, mark, or space")
				}
				called.Parity = value
			case "--stop-bits":
				if kind != transportSerial {
					return invalidInvocation("--stop-bits can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if value != "1" && value != "1.5" && value != "2" {
					return invalidInvocation("--stop-bits takes 1, 1.5, or 2")
				}
				called.StopBits = value
			case "--flow":
				if kind != transportSerial {
					return invalidInvocation("--flow can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if value != "none" && value != "rtscts" && value != "xonxoff" {
					return invalidInvocation("--flow takes none, rtscts, or xonxoff")
				}
				called.Flow = value
			case "--dtr", "--rts":
				if kind != transportSerial {
					return invalidInvocation(name + " can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				enabled, err := onOff(name, value)
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if name == "--dtr" {
					called.DTR = &enabled
				} else {
					called.RTS = &enabled
				}
			case "--break":
				if kind != transportSerial {
					return invalidInvocation("--break can only be used with serial")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Break, err = time.ParseDuration(value)
				if err != nil || called.Break <= 0 || called.Break > 5*time.Second {
					return invalidInvocation("--break takes a duration between 1ns and 5s")
				}
			case "--terminal-type":
				if kind != transportTelnet {
					return invalidInvocation("--terminal-type can only be used with telnet")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if value == "" || len(value) > 64 {
					return invalidInvocation("--terminal-type must contain 1 to 64 bytes")
				}
				called.TerminalType = value
			case "--connect-timeout":
				if kind != transportTelnet {
					return invalidInvocation("--connect-timeout can only be used with telnet")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.ConnectTimeout, err = boundedDuration(name, value)
				if err != nil {
					return invalidInvocation(err.Error())
				}
			case "--expect":
				if !called.Run {
					return invalidInvocation("--expect requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Expect = value
			case "--read-for":
				if !called.Run {
					return invalidInvocation("--read-for requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.ReadFor, err = boundedDuration(name, value)
				if err != nil {
					return invalidInvocation(err.Error())
				}
			case "--timeout":
				if !called.Run {
					return invalidInvocation("--timeout requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Timeout, err = boundedDuration(name, value)
				if err != nil {
					return invalidInvocation(err.Error())
				}
			case "--settle":
				if !called.Run {
					return invalidInvocation("--settle requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.Settle, err = time.ParseDuration(value)
				if err != nil || called.Settle < 0 || called.Settle > streamrun.MaxSettle {
					return invalidInvocation(fmt.Sprintf("--settle takes a duration between 0 and %s", streamrun.MaxSettle))
				}
			case "--max-bytes":
				if !called.Run {
					return invalidInvocation("--max-bytes requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				called.MaxBytes, err = positiveInt(name, value)
				if err != nil || called.MaxBytes > streamrun.MaxMaxBytes {
					return invalidInvocation(fmt.Sprintf("--max-bytes takes a number between 1 and %d", streamrun.MaxMaxBytes))
				}
			case "--line-ending":
				if !called.Run {
					return invalidInvocation("--line-ending requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				ending := streamrun.LineEnding(value)
				if ending != streamrun.EndingNone && ending != streamrun.EndingCR && ending != streamrun.EndingLF && ending != streamrun.EndingCRLF {
					return invalidInvocation("--line-ending takes none, cr, lf, or crlf")
				}
				called.LineEnding = ending
			case "--script":
				if !called.Run {
					return invalidInvocation("--script requires --non-interactive")
				}
				value, err := takeValue()
				if err != nil {
					return invalidInvocation(err.Error())
				}
				if value == "" {
					return invalidInvocation("--script takes a file path or -")
				}
				called.Script = value
			default:
				return invalidInvocation("unknown transport option " + name)
			}
			continue
		}
		if called.Target != "" {
			return invalidInvocation(fmt.Sprintf("unexpected transport argument %q", argument))
		}
		called.Target = argument
	}

	if called.Target == "" {
		return invalidInvocation(string(kind) + " requires a target")
	}
	if called.Target[0] == '-' {
		return invalidInvocation(string(kind) + " target must not start with -")
	}
	if !called.Run {
		if called.JSON || called.Expect != "" || called.ReadFor != 0 || called.Script != "" {
			return invalidInvocation("automation options require --non-interactive")
		}
		return invocation{Kind: invocationTransport, Transport: &called}, nil
	}
	if called.Script != "" {
		if delimited || len(called.Command) != 0 || called.Expect != "" || called.ReadFor != 0 {
			return invalidInvocation("--script cannot be combined with command, --expect, or --read-for")
		}
	} else {
		if !delimited || len(called.Command) == 0 {
			return invalidInvocation("non-interactive transport requires -- followed by text, or --script")
		}
		if (called.Expect == "") == (called.ReadFor == 0) {
			return invalidInvocation("non-interactive transport requires exactly one of --expect or --read-for")
		}
		if len(strings.Join(called.Command, " ")) > streamrun.MaxSendBytes {
			return invalidInvocation(fmt.Sprintf("command exceeds %d bytes", streamrun.MaxSendBytes))
		}
	}
	return invocation{Kind: invocationRunTransport, Transport: &called}, nil
}

func positiveInt(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s takes a positive number", name)
	}
	return parsed, nil
}

func boundedDuration(name, value string) (time.Duration, error) {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 || parsed > streamrun.MaxTimeout {
		return 0, fmt.Errorf("%s takes a duration between 1ns and %s", name, streamrun.MaxTimeout)
	}
	return parsed, nil
}

func onOff(name, value string) (bool, error) {
	switch value {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("%s takes on or off", name)
	}
}
