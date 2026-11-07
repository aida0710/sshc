package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenamingAConnectionRenamesTheJumpHopsThatNameIt(t *testing.T) {
	service, workspace := newTestService(t)
	write := func(relative, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workspace.Root(), filepath.FromSlash(relative)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("config", serviceMainConfig+"\nHost near\n\tProxyJump bastion\n")
	write("conf.d/20-jumps.conf", "Host inner\n\tProxyJump bastion\n\nHost chain\n\tProxyJump ops@bastion:2222,nas,ssh://Bastion\n\nHost lookalike\n\tProxyJump bastions\n")

	request := EditRequest{Kind: EditRename, Path: "config", Base: readFile(t, workspace, "config"), Alias: "bastion", NewAlias: "jump"}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	var previewed []string
	for _, diff := range preview.Diffs {
		previewed = append(previewed, diff.Path)
	}
	if !strings.Contains(strings.Join(previewed, " "), "conf.d/20-jumps.conf") {
		t.Errorf("preview diffs = %v, want the file whose jump hops change too", previewed)
	}
	if _, err := service.Save(request); err != nil {
		t.Fatal(err)
	}

	config := readFile(t, workspace, "config")
	if !strings.Contains(config, "Host jump\n") || !strings.Contains(config, "\tProxyJump jump\n") {
		t.Errorf("config after the rename:\n%s", config)
	}
	jumps := readFile(t, workspace, "conf.d/20-jumps.conf")
	for _, want := range []string{"\tProxyJump jump\n", "\tProxyJump ops@jump:2222,nas,ssh://jump\n", "\tProxyJump bastions\n"} {
		if !strings.Contains(jumps, want) {
			t.Errorf("conf.d/20-jumps.conf lacks %q after the rename:\n%s", want, jumps)
		}
	}
}

func TestOnlyTheHopNamedByTheOldAliasIsRenamed(t *testing.T) {
	for _, test := range []struct {
		value, want string
		renamed     bool
	}{
		{"bastion", "jump", true},
		{"BASTION", "jump", true},
		{"ops@bastion:2222", "ops@jump:2222", true},
		{"first,bastion,last", "first,jump,last", true},
		{"ssh://ops@bastion:22", "ssh://ops@jump:22", true},
		{"bastions", "bastions", false},
		{"[2001:db8::1]:22", "[2001:db8::1]:22", false},
		{"none", "none", false},
	} {
		got, renamed := renamedHops(test.value, "bastion", "jump")
		if got != test.want || renamed != test.renamed {
			t.Errorf("renamedHops(%q) = %q, %v; want %q, %v", test.value, got, renamed, test.want, test.renamed)
		}
	}
}
