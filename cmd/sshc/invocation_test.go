package main

import "testing"

func TestParseInvocationSeparatesOwnersFromDesktopActivation(t *testing.T) {
	tests := []struct {
		argv []string
		kind invocationKind
		args []string
	}{
		{[]string{"sshc"}, invocationDesktop, nil},
		{[]string{"sshc", "engine"}, invocationEngine, nil},
		{[]string{"sshc", "headless"}, invocationHeadless, nil},
		{[]string{"sshc", "server-a"}, invocationConnect, []string{"server-a"}},
		{[]string{"sshc", "connect"}, invocationChoose, nil},
		{[]string{"sshc", "connect", "prod"}, invocationChoose, []string{"prod"}},
		{[]string{"sshc", "list"}, invocationList, nil},
		{[]string{"sshc", "open"}, invocationOpen, nil},
		{[]string{"sshc", "status"}, invocationStatus, nil},
		{[]string{"sshc", "vault", "unlock"}, invocationVault, []string{"unlock"}},
		{[]string{"sshc", "help"}, invocationHelp, nil},
		{[]string{"sshc", "-h"}, invocationHelp, nil},
		{[]string{"sshc", "--help"}, invocationHelp, nil},
	}
	for _, test := range tests {
		got, err := parseInvocation(test.argv)
		if err != nil || got.Kind != test.kind || !sameStrings(got.Args, test.args) {
			t.Fatalf("parseInvocation(%q) = %#v, %v; want kind %v and args %q", test.argv, got, err, test.kind, test.args)
		}
	}
}

func TestParseInvocationRejectsArgumentsThatDoNotNameACommand(t *testing.T) {
	tests := [][]string{
		{"sshc", "--own-engine"},
		{"sshc", "engine", "extra"},
		{"sshc", "vault", "rotate"},
		{"sshc", "server-a", "extra"},
	}
	for _, argv := range tests {
		if got, err := parseInvocation(argv); err == nil || got.Kind != invocationInvalid {
			t.Errorf("parseInvocation(%q) = %#v, %v; want usage error", argv, got, err)
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
