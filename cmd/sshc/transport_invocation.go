package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"sshc/internal/streamrun"
	"sshc/internal/terminal"
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

const (
	// defaultTelnetConnectTimeout は、Telnet の TCP 接続を確立するまでの既定の上限
	// である。届く相手なら数秒以内に終わる。CLI の利用者やスクリプトはその場で結果を
	// 待っているので、telnet.DefaultDialTimeout（30 秒）より短く打ち切る。
	defaultTelnetConnectTimeout = 10 * time.Second
	// defaultTransportSettle は、出力が途切れてから続きが無いと見なすまでの既定の
	// 時間である。9600 baud でもおよそ 100 バイト届く長さで、行の途中で途切れたと
	// 誤りにくく、1 回のやり取りも遅くしすぎない。
	defaultTransportSettle = 120 * time.Millisecond
	// maxSerialBreak は --break の上限である。単位の打ち間違い（500ms のつもりの
	// 500s など）で回線を長く止めないためのもの。
	maxSerialBreak = 5 * time.Second
)

func defaultTransportInvocation(kind transportKind, run bool) transportInvocation {
	called := transportInvocation{
		Transport:      kind,
		Run:            run,
		Timeout:        streamrun.DefaultTimeout,
		ConnectTimeout: defaultTelnetConnectTimeout,
		MaxBytes:       streamrun.DefaultMaxBytes,
		Baud:           9600,
		DataBits:       8,
		Parity:         "none",
		StopBits:       "1",
		Flow:           "none",
		TerminalType:   terminal.DefaultTerminalType,
		Encoding:       textencoding.UTF8,
		Settle:         defaultTransportSettle,
	}
	if kind == transportSerial {
		called.LineEnding = streamrun.EndingCR
	} else {
		called.LineEnding = streamrun.EndingCRLF
	}
	return called
}

var (
	transportSerialOptions = []commandOption{
		valueOption("--baud"), valueOption("--data-bits"), valueOption("--parity"), valueOption("--stop-bits"),
		valueOption("--flow"), valueOption("--dtr"), valueOption("--rts"), valueOption("--break"),
	}
	transportTelnetOptions = []commandOption{valueOption("--terminal-type"), valueOption("--connect-timeout")}
	// transportAutomationOptions は、--non-interactive の自動処理だけが使うオプションである。
	transportAutomationOptions = []commandOption{
		switchOption("--require-output"), valueOption("--expect"), valueOption("--read-for"), valueOption("--timeout"),
		valueOption("--settle"), valueOption("--max-bytes"), valueOption("--line-ending"), valueOption("--script"),
	}
)

// transportOptions は、serial と telnet のどちらにも、両方のオプションの一覧を渡す。
// もう片方のオプションを「使えない」と名指しで断るためである。
func transportOptions(kind transportKind) commandOptions {
	options := []commandOption{switchOption("--non-interactive"), jsonOption, valueOption("--encoding")}
	options = append(options, transportSerialOptions...)
	options = append(options, transportTelnetOptions...)
	options = append(options, transportAutomationOptions...)
	return commandOptions{command: string(kind), options: options, endsAtDelimiter: true}
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

	arguments, err := transportOptions(kind).parse(args)
	if err != nil {
		return invalidInvocation(err.Error())
	}
	if err := readTransportOptions(arguments, &called); err != nil {
		return invalidInvocation(err.Error())
	}
	if len(arguments.positionals) > 1 {
		return invalidInvocation(fmt.Sprintf("unexpected transport argument %q", arguments.positionals[1]))
	}
	if len(arguments.positionals) == 0 || arguments.positionals[0] == "" {
		return invalidInvocation(string(kind) + " requires a target")
	}
	called.Target = arguments.positionals[0]
	if called.Target[0] == '-' {
		return invalidInvocation(string(kind) + " target must not start with -")
	}

	if !called.Run {
		if called.JSON || arguments.delimited {
			return invalidInvocation("automation options require --non-interactive")
		}
		return invocation{Kind: invocationTransport, Transport: &called}, nil
	}
	called.Command = arguments.rest
	if called.Script != "" {
		if arguments.delimited || len(called.Command) != 0 || called.Expect != "" || called.ReadFor != 0 {
			return invalidInvocation("--script cannot be combined with command, --expect, or --read-for")
		}
	} else {
		if !arguments.delimited || len(called.Command) == 0 {
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

// readTransportOptions は、読み取ったオプションの値を確かめて called へ入れる。
func readTransportOptions(arguments commandArguments, called *transportInvocation) error {
	called.Run = arguments.has("--non-interactive")
	called.JSON = arguments.has("--json")
	if value, found := arguments.value("--encoding"); found {
		encoding, err := textencoding.Parse(value)
		if err != nil {
			return fmt.Errorf("--encoding takes utf-8, shift_jis, euc-jp, or iso-2022-jp")
		}
		called.Encoding = encoding
	}
	if err := refuseTransportOptions(arguments, transportSerialOptions,
		called.Transport != transportSerial, "can only be used with serial"); err != nil {
		return err
	}
	if err := refuseTransportOptions(arguments, transportTelnetOptions,
		called.Transport != transportTelnet, "can only be used with telnet"); err != nil {
		return err
	}
	if err := refuseTransportOptions(arguments, transportAutomationOptions,
		!called.Run, "requires --non-interactive"); err != nil {
		return err
	}
	if err := readSerialOptions(arguments, called); err != nil {
		return err
	}
	if err := readTelnetOptions(arguments, called); err != nil {
		return err
	}
	return readTransportAutomationOptions(arguments, called)
}

// refuseTransportOptions は、refused が true のとき、options のうち指定されたものを reason で断る。
func refuseTransportOptions(arguments commandArguments, options []commandOption, refused bool, reason string) error {
	if !refused {
		return nil
	}
	for _, option := range options {
		if arguments.has(option.name) {
			return fmt.Errorf("%s %s", option.name, reason)
		}
	}
	return nil
}

func readSerialOptions(arguments commandArguments, called *transportInvocation) error {
	if value, found := arguments.value("--baud"); found {
		baud, err := positiveInt("--baud", value)
		if err != nil || baud > 4_000_000 {
			return fmt.Errorf("--baud takes a number between 1 and 4000000")
		}
		called.Baud = baud
	}
	if value, found := arguments.value("--data-bits"); found {
		dataBits, err := strconv.Atoi(value)
		if err != nil || dataBits < 5 || dataBits > 8 {
			return fmt.Errorf("--data-bits takes 5, 6, 7, or 8")
		}
		called.DataBits = dataBits
	}
	if value, found := arguments.value("--parity"); found {
		if value != "none" && value != "odd" && value != "even" && value != "mark" && value != "space" {
			return fmt.Errorf("--parity takes none, odd, even, mark, or space")
		}
		called.Parity = value
	}
	if value, found := arguments.value("--stop-bits"); found {
		if value != "1" && value != "1.5" && value != "2" {
			return fmt.Errorf("--stop-bits takes 1, 1.5, or 2")
		}
		called.StopBits = value
	}
	if value, found := arguments.value("--flow"); found {
		if value != "none" && value != "rtscts" && value != "xonxoff" {
			return fmt.Errorf("--flow takes none, rtscts, or xonxoff")
		}
		called.Flow = value
	}
	for _, signal := range []struct {
		name   string
		target **bool
	}{{"--dtr", &called.DTR}, {"--rts", &called.RTS}} {
		value, found := arguments.value(signal.name)
		if !found {
			continue
		}
		enabled, err := onOff(signal.name, value)
		if err != nil {
			return err
		}
		*signal.target = &enabled
	}
	if value, found := arguments.value("--break"); found {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 || duration > maxSerialBreak {
			return fmt.Errorf("--break takes a duration between 1ns and %s", maxSerialBreak)
		}
		called.Break = duration
	}
	return nil
}

func readTelnetOptions(arguments commandArguments, called *transportInvocation) error {
	if value, found := arguments.value("--terminal-type"); found {
		if value == "" || len(value) > 64 {
			return fmt.Errorf("--terminal-type must contain 1 to 64 bytes")
		}
		called.TerminalType = value
	}
	if value, found := arguments.value("--connect-timeout"); found {
		timeout, err := boundedDuration("--connect-timeout", value)
		if err != nil {
			return err
		}
		called.ConnectTimeout = timeout
	}
	return nil
}

func readTransportAutomationOptions(arguments commandArguments, called *transportInvocation) error {
	called.RequireOutput = arguments.has("--require-output")
	called.Expect, _ = arguments.value("--expect")
	for _, limit := range []struct {
		name   string
		target *time.Duration
	}{{"--read-for", &called.ReadFor}, {"--timeout", &called.Timeout}} {
		value, found := arguments.value(limit.name)
		if !found {
			continue
		}
		duration, err := boundedDuration(limit.name, value)
		if err != nil {
			return err
		}
		*limit.target = duration
	}
	if value, found := arguments.value("--settle"); found {
		settle, err := time.ParseDuration(value)
		if err != nil || settle < 0 || settle > streamrun.MaxSettle {
			return fmt.Errorf("--settle takes a duration between 0 and %s", streamrun.MaxSettle)
		}
		called.Settle = settle
	}
	if value, found := arguments.value("--max-bytes"); found {
		maxBytes, err := positiveInt("--max-bytes", value)
		if err != nil || maxBytes > streamrun.MaxMaxBytes {
			return fmt.Errorf("--max-bytes takes a number between 1 and %d", streamrun.MaxMaxBytes)
		}
		called.MaxBytes = maxBytes
	}
	if value, found := arguments.value("--line-ending"); found {
		ending := streamrun.LineEnding(value)
		if ending != streamrun.EndingNone && ending != streamrun.EndingCR && ending != streamrun.EndingLF && ending != streamrun.EndingCRLF {
			return fmt.Errorf("--line-ending takes none, cr, lf, or crlf")
		}
		called.LineEnding = ending
	}
	if value, found := arguments.value("--script"); found {
		if value == "" {
			return fmt.Errorf("--script takes a file path or -")
		}
		called.Script = value
	}
	return nil
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
