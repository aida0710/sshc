package main

import (
	"reflect"
	"testing"
	"time"

	"sshc/internal/streamrun"
	"sshc/internal/textencoding"
)

func TestParseSerialListJSON(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "serial", "list", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if called.Kind != invocationTransport || called.Transport == nil || !called.Transport.List || !called.Transport.JSON {
		t.Fatalf("invocation = %#v", called)
	}
}

func TestParseInteractiveSerialOptions(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "serial", "/dev/ttyUSB0", "--baud=9600", "--data-bits", "7", "--parity", "mark", "--stop-bits", "1.5", "--dtr", "off", "--rts", "on", "--break", "250ms"})
	if err != nil {
		t.Fatal(err)
	}
	got := called.Transport
	if called.Kind != invocationTransport || got == nil || got.Target != "/dev/ttyUSB0" || got.Baud != 9600 || got.DataBits != 7 || got.Parity != "mark" || got.StopBits != "1.5" || got.DTR == nil || *got.DTR || got.RTS == nil || !*got.RTS || got.Break != 250*time.Millisecond {
		t.Fatalf("transport = %#v", got)
	}
}

func TestParseSerialDefaultsMatchNetworkConsole(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "serial", "COM3"})
	if err != nil {
		t.Fatal(err)
	}
	if got := called.Transport; got == nil || got.Baud != 9600 || got.DataBits != 8 || got.Parity != "none" || got.StopBits != "1" {
		t.Fatalf("transport = %#v", got)
	}
}

func TestParseTelnetUsesOneShotDefaults(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "telnet", "switch.example:2323", "--connect-timeout", "3s"})
	if err != nil {
		t.Fatal(err)
	}
	got := called.Transport
	if got == nil || got.Transport != transportTelnet || got.Target != "switch.example:2323" || got.ConnectTimeout != 3*time.Second || got.TerminalType != "xterm-256color" {
		t.Fatalf("transport = %#v", got)
	}
}

func TestParseTransportEncodingCanonicalisesCommonNames(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "telnet", "legacy.example", "--encoding", "sjis"})
	if err != nil {
		t.Fatal(err)
	}
	if got := called.Transport; got == nil || got.Encoding != textencoding.ShiftJIS {
		t.Fatalf("transport = %#v", got)
	}
	if _, err := parseInvocation([]string{"sshc", "serial", "COM3", "--encoding", "unknown"}); err == nil {
		t.Fatal("unsupported encoding was accepted")
	}
}

func TestParseRunSerialKeepsTextAfterDelimiter(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "run", "serial", "COM3", "--expect", `router# $`, "--line-ending", "crlf", "--", "show", "version"})
	if err != nil {
		t.Fatal(err)
	}
	got := called.Transport
	if called.Kind != invocationRunTransport || got == nil || !got.Run || got.Expect != `router# $` || got.LineEnding != streamrun.EndingCRLF || !reflect.DeepEqual(got.Command, []string{"show", "version"}) {
		t.Fatalf("transport = %#v", got)
	}
}

func TestParseRunTelnetScript(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "run", "telnet", "router", "--script", "-", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if got := called.Transport; got == nil || got.Script != "-" || !got.JSON || got.LineEnding != streamrun.EndingCRLF {
		t.Fatalf("transport = %#v", got)
	}
}

func TestParseExplicitSSHAndPreserveLegacyRunDelimiter(t *testing.T) {
	explicit, err := parseInvocation([]string{"sshc", "run", "ssh", "serial", "--", "uname", "-a"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.Kind != invocationRun || !reflect.DeepEqual(explicit.Args, []string{"serial", "uname", "-a"}) {
		t.Fatalf("explicit = %#v", explicit)
	}
	legacy, err := parseInvocation([]string{"sshc", "run", "production", "--", "uname", "-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy.Args, []string{"production", "--", "uname", "-a"}) {
		t.Fatalf("legacy = %#v", legacy)
	}
	interactive, err := parseInvocation([]string{"sshc", "ssh", "telnet"})
	if err != nil || interactive.Kind != invocationConnect || !reflect.DeepEqual(interactive.Args, []string{"telnet"}) {
		t.Fatalf("interactive = %#v, %v", interactive, err)
	}
}

func TestTransportParserRejectsAmbiguousOrUnsafeShapes(t *testing.T) {
	cases := [][]string{
		{"sshc", "serial"},
		{"sshc", "telnet"},
		{"sshc", "run", "serial", "COM3", "--", "show"},
		{"sshc", "run", "telnet", "router", "--expect", "#", "--read-for", "1s", "--", "show"},
		{"sshc", "run", "serial", "COM3", "--expect", "#", "--expect", ">", "--", "show"},
		{"sshc", "telnet", "router", "--baud", "9600"},
		{"sshc", "serial", "COM3", "--line-ending", "lf"},
		{"sshc", "run", "ssh", "host", "uname"},
	}
	for _, argv := range cases {
		if _, err := parseInvocation(argv); err == nil {
			t.Errorf("parseInvocation(%q) succeeded", argv)
		}
	}
}
