package main

import (
	"strings"
	"testing"
)

func TestOpenAndStopHaveMatchingParsingHelpAndCompletion(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		kind      invocationKind
		noBrowser bool
	}{
		{[]string{"sshc"}, invocationOpenInBrowser, false},
		{[]string{"sshc", "open"}, invocationOpen, false},
		{[]string{"sshc", "open", "--no-browser"}, invocationOpen, true},
		{[]string{"sshc", "stop"}, invocationStop, false},
	} {
		called, err := parseInvocation(test.arguments)
		if err != nil || called.Kind != test.kind || called.NoBrowser != test.noBrowser || len(called.Args) != 0 {
			t.Fatalf("%v: %+v, %v", test.arguments, called, err)
		}
	}
	for _, arguments := range [][]string{
		{"sshc", "open", "--no-browser", "--no-browser"}, {"sshc", "open", "r540", "other"},
		{"sshc", "open", "--json"}, {"sshc", "stop", "extra"},
	} {
		if _, err := parseInvocation(arguments); err == nil {
			t.Fatalf("accepted %v", arguments)
		}
	}
	for _, topic := range []string{"open", "stop"} {
		var help strings.Builder
		usageFor(&help, topic)
		if !strings.Contains(help.String(), "sshc "+topic) || (topic == "open" && !strings.Contains(help.String(), "--no-browser")) {
			t.Fatalf("help: %q", help.String())
		}
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var completion strings.Builder
		if err := writeCompletion(&completion, shell); err != nil {
			t.Fatal(err)
		}
		for _, word := range []string{"stop", "--no-browser"} {
			if !strings.Contains(completion.String(), word) {
				t.Fatalf("%s completion omits %s", shell, word)
			}
		}
	}
}

func TestOpenWithAnAliasOpensThatHostsEngineWithOrWithoutABrowser(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		noBrowser bool
	}{
		{[]string{"sshc", "open", "r540"}, false},
		{[]string{"sshc", "open", "r540", "--no-browser"}, true},
		{[]string{"sshc", "open", "--no-browser", "r540"}, true},
	} {
		called, err := parseInvocation(test.arguments)
		if err != nil || called.Kind != invocationOpen || called.NoBrowser != test.noBrowser ||
			len(called.Args) != 1 || called.Args[0] != "r540" {
			t.Fatalf("%v: %+v, %v", test.arguments, called, err)
		}
	}
}

func TestARefusedRemoteEngineIsExplainedWithTheNextStep(t *testing.T) {
	for _, test := range []struct {
		refusal engineProblem
		advice  string
	}{
		{engineProblem{Code: "remote_engine_connect_failed", Reason: "host_key_unknown"}, "sshc ssh r540"},
		{engineProblem{Code: "remote_engine_connect_failed", Reason: "prompt_unavailable"}, "sshc vault unlock"},
		{engineProblem{Code: "remote_engine_connect_failed", Reason: "connection_failed"}, "sshc ssh r540"},
		{engineProblem{Code: "remote_engine_not_installed"}, "install sshc there"},
		{engineProblem{Code: "remote_engine_no_entrance"}, "sshc open --no-browser on r540"},
		{engineProblem{Code: "remote_engine_forwarding_refused"}, "AllowTcpForwarding"},
		{engineProblem{Code: "server_stopping"}, "code server_stopping"},
	} {
		if got := remoteEngineRefusal("r540", test.refusal); !strings.Contains(got, test.advice) {
			t.Errorf("%+v = %q, want it to mention %q", test.refusal, got, test.advice)
		}
	}
}

func TestWindowsUnreachableAdviceDoesNotMentionUnixCommands(t *testing.T) {
	advice := engineNotRespondingAdvice("windows")
	if strings.Contains(advice, "fg") || !strings.Contains(advice, "sshc service restart") {
		t.Fatalf("Windows advice = %q", advice)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if !strings.Contains(engineNotRespondingAdvice(goos), "fg") {
			t.Fatalf("%s advice omitted suspended engine recovery", goos)
		}
	}
}

func TestEngineBackgroundFlagIsParsedAndDocumented(t *testing.T) {
	called, err := parseInvocation([]string{"sshc", "engine", "--background", "--port", "54448"})
	if err != nil || !called.Background || called.Port != 54448 {
		t.Fatalf("invocation=%+v err=%v", called, err)
	}
	if _, err := parseInvocation([]string{"sshc", "engine", "--background", "--background"}); err == nil {
		t.Fatal("duplicate background accepted")
	}
	var help strings.Builder
	usageFor(&help, "engine")
	if !strings.Contains(help.String(), "--background") || !strings.Contains(help.String(), "services") {
		t.Fatalf("help=%q", help.String())
	}
}
