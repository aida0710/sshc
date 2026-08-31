package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionScriptsReadCurrentSSHAliases(t *testing.T) {
	tests := map[string][]string{
		"bash": {"complete -F _sshc_completion sshc", "command sshc ssh --list"},
		"zsh":  {"#compdef sshc", "compdef _sshc sshc", "command sshc ssh --list"},
		"fish": {"complete -c sshc", "command sshc ssh --list"},
	}
	for shell, required := range tests {
		t.Run(shell, func(t *testing.T) {
			var output bytes.Buffer
			if err := writeCompletion(&output, shell); err != nil {
				t.Fatal(err)
			}
			for _, fragment := range required {
				if !strings.Contains(output.String(), fragment) {
					t.Errorf("%s completion lacks %q", shell, fragment)
				}
			}
		})
	}
}

func TestCompletionRejectsAnUnsupportedShell(t *testing.T) {
	if err := writeCompletion(&bytes.Buffer{}, "powershell"); err == nil {
		t.Fatal("unsupported shell was accepted")
	}
}
