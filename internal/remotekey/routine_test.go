package remotekey_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/remotekey"
)

// existingKeyLine は、登録先にもともとある別の鍵の行である。
const existingKeyLine = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExistingExistingExistingExistingExisting0 old@example"

// runRoutine は Routine を、HOME を一時ディレクトリにした sh で実行し、
// 標準出力を返す。リモートの POSIX シェルの代わりにローカルの sh を使う。
func runRoutine(t *testing.T, home string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the remote routine targets a POSIX shell")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	command := exec.Command(shell, "-c", remotekey.Routine)
	command.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
	command.Stdin = strings.NewReader(keyLine + "\n")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("routine = %v, output %q", err, output)
	}
	return string(output)
}

// writeAuthorizedKeys は、登録先の authorized_keys をあらかじめ用意する。
func writeAuthorizedKeys(t *testing.T, home, content string) string {
	t.Helper()
	path := filepath.Join(home, ".ssh", "authorized_keys")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readAuthorizedKeys(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestRoutineAppendsTheKeyOnItsOwnLine(t *testing.T) {
	commentlessExistingLine := strings.TrimSuffix(existingKeyLine, " old@example")
	cases := []struct {
		name     string
		existing string
		want     string
	}{
		{name: "empty file", existing: "", want: keyLine + "\n"},
		{name: "file ending with a newline", existing: existingKeyLine + "\n", want: existingKeyLine + "\n" + keyLine + "\n"},
		{name: "last line with a comment and no newline", existing: existingKeyLine, want: existingKeyLine + "\n" + keyLine + "\n"},
		{name: "last line without a comment and no newline", existing: commentlessExistingLine, want: commentlessExistingLine + "\n" + keyLine + "\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			home := t.TempDir()
			path := writeAuthorizedKeys(t, home, testCase.existing)

			if output := runRoutine(t, home); !strings.Contains(output, "sshc: added") {
				t.Fatalf("output = %q", output)
			}
			if got := readAuthorizedKeys(t, path); got != testCase.want {
				t.Fatalf("authorized_keys = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestRoutineCreatesAMissingAuthorizedKeysFile(t *testing.T) {
	home := t.TempDir()

	if output := runRoutine(t, home); !strings.Contains(output, "sshc: added") {
		t.Fatalf("output = %q", output)
	}
	path := filepath.Join(home, ".ssh", "authorized_keys")
	if got := readAuthorizedKeys(t, path); got != keyLine+"\n" {
		t.Fatalf("authorized_keys = %q", got)
	}
}

func TestRoutineRunTwiceAddsTheKeyOnlyOnce(t *testing.T) {
	home := t.TempDir()
	path := writeAuthorizedKeys(t, home, existingKeyLine)

	runRoutine(t, home)
	if output := runRoutine(t, home); !strings.Contains(output, "sshc: already-present") {
		t.Fatalf("second run output = %q", output)
	}
	if got, want := readAuthorizedKeys(t, path), existingKeyLine+"\n"+keyLine+"\n"; got != want {
		t.Fatalf("authorized_keys = %q, want %q", got, want)
	}
}

func TestRoutineTreatsTheSameKeyWithAnotherCommentOrOptionsAsPresent(t *testing.T) {
	keyWithoutComment := strings.TrimSuffix(keyLine, " fixture@example")
	cases := map[string]string{
		"another comment": keyWithoutComment + " laptop@example\n",
		"no comment":      keyWithoutComment + "\n",
		"options":         `from="192.0.2.0/24",command="uptime" ` + keyLine + "\n",
		"tab separated":   strings.Replace(keyWithoutComment, " ", "\t", 1) + "\n",
	}
	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			path := writeAuthorizedKeys(t, home, existing)

			if output := runRoutine(t, home); !strings.Contains(output, "sshc: already-present") {
				t.Fatalf("output = %q", output)
			}
			if got := readAuthorizedKeys(t, path); got != existing {
				t.Fatalf("authorized_keys changed to %q", got)
			}
		})
	}
}

func TestRoutineAppendsTheKeyEvenWhenTheSameKeyIsCommentedOut(t *testing.T) {
	cases := map[string]string{
		"comment at the start":     "# " + keyLine + " disabled\n",
		"comment after whitespace": "  \t# " + keyLine + "\n",
		"comment without a space":  "#" + keyLine + "\n",
	}
	for name, existing := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			path := writeAuthorizedKeys(t, home, existing)

			if output := runRoutine(t, home); !strings.Contains(output, "sshc: added") {
				t.Fatalf("output = %q", output)
			}
			if got, want := readAuthorizedKeys(t, path), existing+keyLine+"\n"; got != want {
				t.Fatalf("authorized_keys = %q, want %q", got, want)
			}
		})
	}
}
