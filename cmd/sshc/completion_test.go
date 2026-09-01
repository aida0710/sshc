package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionScriptsReadCurrentSSHAliases(t *testing.T) {
	tests := map[string][]string{
		"bash": {"complete -F _sshc_completion sshc", "command sshc ssh --list", "service"},
		"zsh":  {"#compdef sshc", "compdef _sshc sshc", "command sshc ssh --list", "service"},
		"fish": {"complete -c sshc", "command sshc ssh --list", "service"},
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

func TestEveryShellCompletesThePublishedCommandTree(t *testing.T) {
	grammar := cliCompletionGrammar
	required := []string{
		strings.Join(grammar.topLevel, " "),
		strings.Join(grammar.syncActions, " "),
		strings.Join(grammar.terminalActions, " "),
		strings.Join(grammar.vaultActions, " "),
		strings.Join(grammar.serviceActions, " "),
		strings.Join(grammar.encodings, " "),
		strings.Join(grammar.waitStates, " "),
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			var output bytes.Buffer
			if err := writeCompletion(&output, shell); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "{{") {
				t.Fatal("completion contains an unresolved grammar placeholder")
			}
			for _, fragment := range required {
				if !strings.Contains(output.String(), fragment) {
					t.Errorf("%s completion lacks command tree %q", shell, fragment)
				}
			}
		})
	}
}

func TestCompletionGrammarOnlyPublishesValidHelpTopics(t *testing.T) {
	grammar := cliCompletionGrammar
	for _, topic := range grammar.helpTopics {
		if !validHelpTopic(topic) {
			t.Errorf("completion publishes unknown help topic %q", topic)
		}
	}
	for parent, actions := range map[string][]string{
		"sync": grammar.syncActions, "terminal": grammar.terminalActions,
		"service": grammar.serviceActions, "vault": grammar.vaultActions,
	} {
		for _, action := range actions {
			topic := parent + " " + action
			if !validHelpTopic(topic) {
				t.Errorf("completion publishes unknown help topic %q", topic)
			}
		}
	}
}

func TestBashCompletionUsesLiveAliasesAndNestedValues(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	directory := t.TempDir()
	completionPath := filepath.Join(directory, "sshc-completion.bash")
	if err := os.WriteFile(completionPath, []byte(bashCompletion), 0o600); err != nil {
		t.Fatal(err)
	}
	fakePath := filepath.Join(directory, "sshc")
	fake := "#!/bin/sh\nif [ \"$1 $2\" = \"ssh --list\" ]; then printf 'alpha\\nbeta-prod\\n'; fi\n"
	if err := os.WriteFile(fakePath, []byte(fake), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		words []string
		want  string
	}{
		{name: "ssh alias", words: []string{"sshc", "ssh", "b"}, want: "beta-prod"},
		{name: "info alias", words: []string{"sshc", "info", "a"}, want: "alpha"},
		{name: "terminal alias", words: []string{"sshc", "terminal", "create", "ssh", "b"}, want: "beta-prod"},
		{name: "sync action", words: []string{"sshc", "sync", "p"}, want: "push"},
		{name: "sync auto value", words: []string{"sshc", "sync", "auto", "o"}, want: "on"},
		{name: "terminal state", words: []string{"sshc", "terminal", "wait", "deadbeef", "--for", "agent-r"}, want: "agent-ready"},
		{name: "encoding", words: []string{"sshc", "serial", "/dev/ttyUSB0", "--encoding", "shift"}, want: "shift_jis"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := append([]string{"-c", `source "$COMPLETION"
COMP_WORDS=("$@")
COMP_CWORD=$((${#COMP_WORDS[@]} - 1))
_sshc_completion
printf '%s\n' "${COMPREPLY[@]}"`, "completion-test"}, test.words...)
			command := exec.Command(bash, arguments...)
			command.Env = append(os.Environ(),
				"COMPLETION="+completionPath,
				"PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
			)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("completion failed: %v\n%s", err, output)
			}
			if candidates := strings.Fields(string(output)); !containsCompletion(candidates, test.want) {
				t.Fatalf("completion = %q; want %q", candidates, test.want)
			}
		})
	}
}

// OpenSSH は `Host $(id)` のような alias も読む。`sshc ssh --list` はそれをその
// まま出すので、補完はどの入口でも alias を展開せず、ただの文字列として扱う。
func TestBashCompletionNeverEvaluatesAliasesFromTheSSHConfig(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed")
	}
	directory := t.TempDir()
	completionPath := filepath.Join(directory, "sshc-completion.bash")
	if err := os.WriteFile(completionPath, []byte(bashCompletion), 0o600); err != nil {
		t.Fatal(err)
	}
	substitution := filepath.Join(directory, "substitution")
	backquote := filepath.Join(directory, "backquote")
	aliasesPath := filepath.Join(directory, "aliases.txt")
	aliases := "$(touch " + substitution + ")\n`touch " + backquote + "`\nalpha\n"
	if err := os.WriteFile(aliasesPath, []byte(aliases), 0o600); err != nil {
		t.Fatal(err)
	}
	fakePath := filepath.Join(directory, "sshc")
	// 候補をscriptのprintf formatへ埋め込むと、Windows pathの `\001` などが
	// escapeとして解釈される。候補そのものを評価しないfixtureにするため、別fileを読む。
	fake := "#!/bin/sh\nif [ \"$1 $2\" = \"ssh --list\" ]; then cat \"$SSHC_TEST_ALIASES\"; fi\n"
	if err := os.WriteFile(fakePath, []byte(fake), 0o700); err != nil {
		t.Fatal(err)
	}

	entries := map[string][]string{
		"ssh":             {"sshc", "ssh", ""},
		"info":            {"sshc", "info", ""},
		"terminal create": {"sshc", "terminal", "create", "ssh", ""},
	}
	for name, words := range entries {
		t.Run(name, func(t *testing.T) {
			arguments := append([]string{"-c", `source "$COMPLETION"
COMP_WORDS=("$@")
COMP_CWORD=$((${#COMP_WORDS[@]} - 1))
_sshc_completion
printf '%s\n' "${COMPREPLY[@]}"`, "completion-test"}, words...)
			command := exec.Command(bash, arguments...)
			command.Env = append(os.Environ(),
				"COMPLETION="+completionPath,
				"PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SSHC_TEST_ALIASES="+aliasesPath,
			)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("completion failed: %v\n%s", err, output)
			}
			// 展開されずに読めていることを確かめる。全部落としてしまうと、この
			// テストは何も守らないまま通ってしまう。
			if !strings.Contains(string(output), "$(touch "+substitution+")") {
				t.Fatalf("completion dropped the alias instead of offering it verbatim: %s", output)
			}
		})
	}
	for _, marker := range []string{substitution, backquote} {
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("completion executed the alias and created %s", marker)
		}
	}
}

func containsCompletion(candidates []string, wanted string) bool {
	for _, candidate := range candidates {
		if candidate == wanted {
			return true
		}
	}
	return false
}
