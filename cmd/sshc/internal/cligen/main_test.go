package main

import (
	"bytes"
	"go/format"
	"strings"
	"testing"

	"sshc/cmd/sshc/internal/clispec"
)

func TestGenerateProducesFormattedDeterministicContract(t *testing.T) {
	first, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("CLI contract generation is not deterministic")
	}
	formatted, err := format.Source(first)
	if err != nil || !bytes.Equal(first, formatted) {
		t.Fatalf("generated contract is not gofmt-clean: %v", err)
	}
}

func TestEveryPublishedCommandFeedsDispatchHelpAndCompletion(t *testing.T) {
	contract, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	generated := string(contract)
	for _, command := range clispec.Commands {
		if strings.HasPrefix(command.Name, "-") {
			continue
		}
		if !strings.Contains(generated, `case "`+command.Name+`"`) {
			t.Errorf("generated parser does not dispatch %q", command.Name)
		}
		if command.Help != "" && !strings.Contains(generated, `"`+command.Name+`":`) {
			t.Errorf("generated help does not contain %q", command.Name)
		}
		for _, action := range command.Actions {
			topic := command.Name + " " + action.Name
			if !strings.Contains(generated, `case "`+topic+`"`) || !strings.Contains(generated, `"`+topic+`":`) {
				t.Errorf("generated parser/help does not contain %q", topic)
			}
		}
	}
}

func TestGenerateRejectsCollidingCommandSpellings(t *testing.T) {
	original := clispec.Commands
	t.Cleanup(func() { clispec.Commands = original })
	clispec.Commands = append(append([]clispec.Command(nil), original...), clispec.Command{
		Name:    "collision",
		Route:   "collision",
		Aliases: []string{"--version"},
	})

	_, err := generate()
	if err == nil || !strings.Contains(err.Error(), `command spelling "--version" is shared`) {
		t.Fatalf("generate() error = %v, want duplicate-spelling error", err)
	}
}
