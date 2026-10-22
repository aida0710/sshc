package main

import "testing"

func TestTerminalCreateReadsWorkspaceAnywhereAfterTheTarget(t *testing.T) {
	for _, test := range []struct {
		args      []string
		alias     string
		workspace string
		json      bool
	}{
		{[]string{"shell"}, "", "", false},
		{[]string{"shell", "--workspace", "sshc開発"}, "", "sshc開発", false},
		{[]string{"ssh", "r540", "--json", "--workspace", "default"}, "r540", "default", true},
	} {
		var parsed terminalInvocation
		if err := readTerminalCreate(test.args, &parsed); err != nil {
			t.Fatalf("%v: %v", test.args, err)
		}
		if parsed.Alias != test.alias || parsed.Workspace != test.workspace || parsed.JSON != test.json {
			t.Fatalf("%v: parsed %+v", test.args, parsed)
		}
	}
	for _, args := range [][]string{
		{"shell", "--workspace"},
		{"shell", "--workspace", " "},
		{"shell", "--workspace", "a", "--workspace", "b"},
	} {
		var parsed terminalInvocation
		if err := readTerminalCreate(args, &parsed); err == nil {
			t.Fatalf("%v was accepted", args)
		}
	}
}
