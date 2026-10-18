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
		if err != nil || called.Kind != test.kind || called.NoBrowser != test.noBrowser {
			t.Fatalf("%v: %+v, %v", test.arguments, called, err)
		}
	}
	for _, arguments := range [][]string{{"sshc", "open", "--no-browser", "--no-browser"}, {"sshc", "open", "extra"}, {"sshc", "stop", "extra"}} {
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

func TestWindowsUnreachableAdviceDoesNotMentionUnixCommands(t *testing.T) {
	advice := engineNotRespondingAdvice("windows")
	if strings.Contains(advice, "fg") || strings.Contains(advice, "sshc service") {
		t.Fatalf("Windows advice = %q", advice)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if !strings.Contains(engineNotRespondingAdvice(goos), "fg") {
			t.Fatalf("%s advice omitted suspended engine recovery", goos)
		}
	}
}
