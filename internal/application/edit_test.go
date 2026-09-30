package application

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/config"
)

const editConfig = `Host bastion
	HostName 203.0.113.10
	User ops
	Port 22 # inherited from the old server
	# keep this comment

Host nas
	User aida
`

func parseEditFixture(t *testing.T, source string) (*config.File, config.Block) {
	t.Helper()
	file := config.Parse([]byte(source))
	block, ok := FindHostBlock(file, "bastion")
	if !ok {
		t.Fatal("fixture has no bastion block")
	}
	return file, block
}

func TestApplyFieldEditsChangesOnlyTheEditedLines(t *testing.T) {
	file, block := parseEditFixture(t, editConfig)

	err := ApplyFieldEdits(file, block, []FieldEdit{
		{Action: ActionSet, Line: 4, Values: []string{"2222"}},
		{Action: ActionRemove, Line: 3},
		{Action: ActionAdd, Keyword: "IdentityFile", Values: []string{"~/.ssh/id_ed25519"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	const want = `Host bastion
	HostName 203.0.113.10
	Port 2222 # inherited from the old server
	IdentityFile ~/.ssh/id_ed25519
	# keep this comment

Host nas
	User aida
`
	if got := string(file.Render()); got != want {
		t.Fatalf("render =\n%q\nwant\n%q", got, want)
	}
}

func TestApplyFieldEditsWithNoEditsRendersTheOriginalBytes(t *testing.T) {
	file, block := parseEditFixture(t, editConfig)
	if err := ApplyFieldEdits(file, block, nil); err != nil {
		t.Fatal(err)
	}
	if got := string(file.Render()); got != editConfig {
		t.Fatalf("render = %q", got)
	}
}

func TestApplyFieldEditsQuotesValuesAndRefusesUnrepresentableOnes(t *testing.T) {
	file, block := parseEditFixture(t, editConfig)
	if err := ApplyFieldEdits(file, block, []FieldEdit{
		{Action: ActionAdd, Keyword: "SetEnv", Values: []string{"GREETING=hello world", "EMPTY="}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := string(file.Render()); !strings.Contains(got, "\tSetEnv \"GREETING=hello world\" EMPTY=\n") {
		t.Fatalf("render = %q", got)
	}

	quoted, quotedBlock := parseEditFixture(t, editConfig)
	if err := ApplyFieldEdits(quoted, quotedBlock, []FieldEdit{
		{Action: ActionAdd, Keyword: "User", Values: []string{`it's "me"`}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := string(quoted.Render()); !strings.Contains(got, "\tUser \"it's \\\"me\\\"\"\n") {
		t.Fatalf("render = %q", got)
	}

	fresh, freshBlock := parseEditFixture(t, editConfig)
	if err := ApplyFieldEdits(fresh, freshBlock, []FieldEdit{
		{Action: ActionAdd, Keyword: "RemoteCommand", Values: []string{"echo\nhi"}},
	}); !errors.Is(err, ErrUnquotableValue) {
		t.Fatalf("error = %v, want ErrUnquotableValue", err)
	}
	if got := string(fresh.Render()); got != editConfig {
		t.Fatal("a rejected edit must leave the file untouched")
	}
}

// ProxyCommand などの値を読むのはシェルである。画面で編集しても、一重引用符と
// 引用の中のバックスラッシュは書いたとおりに残る。二重引用符に書き直すと、シェルが
// $1 を展開するようになる。
func TestCommandKeywordsKeepTheirQuotingWhenEditedOnScreen(t *testing.T) {
	const source = "Host bastion\n\tRemoteCommand awk '{print $1}' /etc/passwd # users\n"
	file, block := parseEditFixture(t, source)
	if got := formValues(file.Lines[1]); len(got) != 1 || got[0] != "awk '{print $1}' /etc/passwd # users" {
		t.Fatalf("form values = %q, want the rest of the line as one value", got)
	}

	if err := ApplyFieldEdits(file, block, []FieldEdit{
		{Action: ActionSet, Line: 2, Values: []string{"awk -F: '{print $1}' /etc/passwd # users"}},
		{Action: ActionAdd, Keyword: "ProxyCommand", Values: []string{`sh -c 'exec nc $(cat /tmp/h) %p' # printf 'a\\b'`}},
	}); err != nil {
		t.Fatal(err)
	}
	const want = "Host bastion\n" +
		"\tRemoteCommand awk -F: '{print $1}' /etc/passwd # users\n" +
		"\tProxyCommand sh -c 'exec nc $(cat /tmp/h) %p' # printf 'a\\\\b'\n"
	if got := string(file.Render()); got != want {
		t.Fatalf("render =\n%q\nwant\n%q", got, want)
	}
	reparsed := config.Parse(file.Render())
	if got := formValues(reparsed.Lines[1]); len(got) != 1 || got[0] != "awk -F: '{print $1}' /etc/passwd # users" {
		t.Fatalf("form values after saving = %q", got)
	}
}

// OpenSSH は閉じない引用のある行を設定ごと断り、値の先頭の '=' を区切りとして
// 読み飛ばす。どちらも書いたとおりには読み戻せないので、ファイルを変えずに断る。
func TestCommandKeywordsRefuseValuesTheLineCannotHold(t *testing.T) {
	for _, value := range []string{"sh -c 'echo", "=nc %h %p", "echo\nhi"} {
		file, block := parseEditFixture(t, editConfig)
		err := ApplyFieldEdits(file, block, []FieldEdit{{Action: ActionAdd, Keyword: "ProxyCommand", Values: []string{value}}})
		if !errors.Is(err, ErrUnquotableValue) {
			t.Errorf("ProxyCommand %q: error = %v, want ErrUnquotableValue", value, err)
		}
		if got := string(file.Render()); got != editConfig {
			t.Errorf("ProxyCommand %q: a rejected edit changed the file: %q", value, got)
		}
	}
}

func TestApplyFieldEditsRejectsStructuralAndOutOfBlockEdits(t *testing.T) {
	tests := []struct {
		name string
		edit FieldEdit
		want error
	}{
		{"new Host line", FieldEdit{Action: ActionAdd, Keyword: "Host", Values: []string{"evil"}}, ErrStructuralKeyword},
		{"new Include line", FieldEdit{Action: ActionAdd, Keyword: "Include", Values: []string{"/etc/ssh/ssh_config"}}, ErrStructuralKeyword},
		{"new Match line", FieldEdit{Action: ActionAdd, Keyword: "Match", Values: []string{"all"}}, ErrStructuralKeyword},
		{"empty keyword", FieldEdit{Action: ActionAdd, Values: []string{"x"}}, ErrEmptyKeyword},
		{"keyword with a space", FieldEdit{Action: ActionAdd, Keyword: "User Name", Values: []string{"x"}}, ErrInvalidKeyword},
		{"line in another block", FieldEdit{Action: ActionSet, Line: 8, Values: []string{"root"}}, ErrEditLineOutsideBlock},
		{"the header line", FieldEdit{Action: ActionSet, Line: 1, Values: []string{"other"}}, ErrEditLineOutsideBlock},
		{"a comment line", FieldEdit{Action: ActionSet, Line: 5, Values: []string{"x"}}, ErrEditLineNotDirective},
		{"unknown action", FieldEdit{Action: "replace", Line: 3, Values: []string{"x"}}, ErrUnknownEditAction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, block := parseEditFixture(t, editConfig)
			if err := ApplyFieldEdits(file, block, []FieldEdit{test.edit}); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if got := string(file.Render()); got != editConfig {
				t.Fatalf("a rejected edit changed the file: %q", got)
			}
		})
	}
}

func TestApplyFieldEditsKeepsCarriageReturnLineEndings(t *testing.T) {
	source := "Host bastion\r\n\tUser ops\r\n"
	file, block := parseEditFixture(t, source)
	if err := ApplyFieldEdits(file, block, []FieldEdit{
		{Action: ActionAdd, Keyword: "Port", Values: []string{"2222"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := string(file.Render()); got != "Host bastion\r\n\tUser ops\r\n\tPort 2222\r\n" {
		t.Fatalf("render = %q", got)
	}
}

func TestReplaceBlockSwapsExactlyOneBlock(t *testing.T) {
	file, block := parseEditFixture(t, editConfig)
	if err := ReplaceBlock(file, block, "Host bastion\n\tUser root\n\n"); err != nil {
		t.Fatal(err)
	}
	const want = `Host bastion
	User root

Host nas
	User aida
`
	if got := string(file.Render()); got != want {
		t.Fatalf("render =\n%q\nwant\n%q", got, want)
	}
}

func TestReplaceBlockRefusesRawTextThatMovesTheBlockBoundary(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"no header", "\tUser root\n", ErrRawBlockHeader},
		{"empty", "", ErrRawBlockHeader},
		{"two headers", "Host bastion\n\tUser root\nHost extra\n\tUser other\n", ErrRawBlockStructure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, block := parseEditFixture(t, editConfig)
			if err := ReplaceBlock(file, block, test.raw); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if got := string(file.Render()); got != editConfig {
				t.Fatal("a rejected raw block changed the file")
			}
		})
	}
}

func TestRenameHostAliasRewritesOnlyThePrimaryPattern(t *testing.T) {
	file := config.Parse([]byte("Host bastion jump.example.com # main\n\tUser ops\n"))
	block, ok := FindHostBlock(file, "bastion")
	if !ok {
		t.Fatal("fixture has no bastion block")
	}
	if err := RenameHostAlias(file, block, "bastion", "jump"); err != nil {
		t.Fatal(err)
	}
	if got := string(file.Render()); got != "Host jump jump.example.com # main\n\tUser ops\n" {
		t.Fatalf("render = %q", got)
	}
	for _, alias := range []string{"", "with space", "star*", "!negated", "../escape", "a" + strings.Repeat("b", 100)} {
		if err := ValidateAlias(alias); !errors.Is(err, ErrInvalidAlias) {
			t.Errorf("ValidateAlias(%q) = %v, want ErrInvalidAlias", alias, err)
		}
	}
}
