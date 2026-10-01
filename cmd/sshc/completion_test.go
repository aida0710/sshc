package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sshc/cmd/sshc/internal/clispec"
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

// 補完のスクリプトは利用者のシェルの設定へそのまま入るので、中のコメントも CLI の
// 出力として英語で書く。
func TestCompletionScriptsAreWrittenInEnglish(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var output bytes.Buffer
		if err := writeCompletion(&output, shell); err != nil {
			t.Fatal(err)
		}
		for number, line := range strings.Split(output.String(), "\n") {
			if containsJapanese(line) {
				t.Errorf("%s completion line %d is not English: %q", shell, number+1, line)
			}
		}
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

// specActions は clispec に書いたアクションを、補完の一覧と同じ並びで返す。
// 補完の側（cliActions）ではなく定義の側から数えるので、補完に届かないアクションを見落とさない。
func specActions() map[string][]string {
	actions := map[string][]string{}
	for _, command := range clispec.Commands {
		for _, action := range command.Actions {
			actions[command.Name] = append(actions[command.Name], action.Name)
		}
	}
	return actions
}

func TestCompletionOffersEveryPublishedActionAndOnlyValidHelpTopics(t *testing.T) {
	grammar := cliCompletionGrammar
	for _, topic := range grammar.helpTopics {
		if !validHelpTopic(topic) {
			t.Errorf("completion publishes unknown help topic %q", topic)
		}
	}
	for command, actions := range specActions() {
		for _, action := range actions {
			if !containsCompletion(cliActions[command], action) {
				t.Errorf("completion does not offer %q %q", command, action)
			}
		}
	}
	for command, actions := range cliActions {
		for _, action := range actions {
			if !validHelpTopic(command+" "+action) || !validCLIAction(command, action) {
				t.Errorf("completion publishes unknown action %q %q", command, action)
			}
		}
	}
}

func TestZshAndFishCompleteEveryActionAfterItsCommandAndAfterHelp(t *testing.T) {
	for command := range specActions() {
		actions := strings.Join(cliActions[command], " ")
		for shell, fragments := range map[string][]string{
			"zsh": {
				"_sshc_values '" + actions + " ",
				command + ") _sshc_values '" + actions + "' ;;",
			},
			"fish": {
				"-n '__sshc_prefix " + command + "' -a '" + actions,
				"-n '__sshc_prefix help " + command + "' -a '" + actions + "'",
			},
		} {
			var output bytes.Buffer
			if err := writeCompletion(&output, shell); err != nil {
				t.Fatal(err)
			}
			for _, fragment := range fragments {
				if !strings.Contains(output.String(), fragment) {
					t.Errorf("%s completion for %q lacks %q", shell, command, fragment)
				}
			}
		}
	}
}

func TestBashCompletesEveryActionAfterItsCommandAndAfterHelp(t *testing.T) {
	completion := newBashCompletionFixture(t)
	for command, actions := range specActions() {
		afterCommand := completion.candidates(t, "sshc", command, "")
		afterHelp := completion.candidates(t, "sshc", "help", command, "")
		for _, action := range actions {
			if !containsCompletion(afterCommand, action) {
				t.Errorf("sshc %s <Tab> = %q; want %q", command, afterCommand, action)
			}
			if !containsCompletion(afterHelp, action) {
				t.Errorf("sshc help %s <Tab> = %q; want %q", command, afterHelp, action)
			}
		}
	}
}

// bashCompletionFixture は、補完の雛形と、alias を固定で返す偽の sshc を一時ディレクトリに置く。
type bashCompletionFixture struct {
	bash           string
	completionPath string
	directory      string
}

func newBashCompletionFixture(t *testing.T) bashCompletionFixture {
	t.Helper()
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
	return bashCompletionFixture{bash: bash, completionPath: completionPath, directory: directory}
}

func (fixture bashCompletionFixture) candidates(t *testing.T, words ...string) []string {
	t.Helper()
	arguments := append([]string{"-c", `source "$COMPLETION"
COMP_WORDS=("$@")
COMP_CWORD=$((${#COMP_WORDS[@]} - 1))
_sshc_completion
printf '%s\n' "${COMPREPLY[@]}"`, "completion-test"}, words...)
	command := exec.Command(fixture.bash, arguments...)
	command.Env = append(os.Environ(),
		"COMPLETION="+fixture.completionPath,
		"PATH="+fixture.directory+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("completion failed: %v\n%s", err, output)
	}
	return strings.Fields(string(output))
}

func TestBashCompletionUsesLiveAliasesAndNestedValues(t *testing.T) {
	completion := newBashCompletionFixture(t)
	tests := []struct {
		name  string
		words []string
		want  string
	}{
		{name: "ssh alias", words: []string{"sshc", "ssh", "b"}, want: "beta-prod"},
		{name: "info alias", words: []string{"sshc", "info", "a"}, want: "alpha"},
		{name: "terminal alias", words: []string{"sshc", "terminal", "create", "ssh", "b"}, want: "beta-prod"},
		{name: "vault action", words: []string{"sshc", "vault", "ch"}, want: "change-password"},
		{name: "vault help", words: []string{"sshc", "help", "vault", "un"}, want: "unlock"},
		{name: "sync action", words: []string{"sshc", "sync", "p"}, want: "push"},
		{name: "otp action", words: []string{"sshc", "otp", "e"}, want: "edit"},
		{name: "sync auto value", words: []string{"sshc", "sync", "auto", "o"}, want: "on"},
		{name: "terminal state", words: []string{"sshc", "terminal", "wait", "deadbeef", "--for", "reconn"}, want: "reconnecting"},
		{name: "terminal rename auto", words: []string{"sshc", "terminal", "rename", "deadbeef", "--a"}, want: "--auto"},
		{name: "terminal rename auto json", words: []string{"sshc", "terminal", "rename", "deadbeef", "--auto", "--j"}, want: "--json"},
		{name: "terminal rename title json", words: []string{"sshc", "terminal", "rename", "deadbeef", "deploy", "--j"}, want: "--json"},
		{name: "sftp alias", words: []string{"sshc", "sftp", "get", "b"}, want: "beta-prod"},
		{name: "sftp settings option", words: []string{"sshc", "sftp", "settings", "--split"}, want: "--split-size"},
		{name: "encoding", words: []string{"sshc", "serial", "/dev/ttyUSB0", "--encoding", "shift"}, want: "shift_jis"},
		{name: "vpn list json", words: []string{"sshc", "vpn", "--j"}, want: "--json"},
		{name: "vpn bind alias", words: []string{"sshc", "vpn", "bind", "b"}, want: "beta-prod"},
		{name: "vpn unbind alias", words: []string{"sshc", "vpn", "unbind", "a"}, want: "alpha"},
		{name: "vpn remove yes", words: []string{"sshc", "vpn", "remove", "office", "--y"}, want: "--yes"},
		{name: "vpn up json", words: []string{"sshc", "vpn", "up", "office", "--j"}, want: "--json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if candidates := completion.candidates(t, test.words...); !containsCompletion(candidates, test.want) {
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
