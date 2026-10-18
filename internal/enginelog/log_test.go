package enginelog

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
)

func TestOpenAppendsAndRotatesOnlyAboveTheLimit(t *testing.T) {
	for _, size := range []int{MaximumSize, MaximumSize + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			stateDir := t.TempDir()
			path := Path(stateDir)
			acltest.WritePrivateFile(t, path, []byte(strings.Repeat("x", size)))
			acltest.WritePrivateFile(t, path+".1", []byte("previous rotation"))
			file, err := Open(stateDir, "test-version")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString("child output\n"); err != nil {
				t.Fatal(err)
			}
			file.Close()
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "=== sshc test-version engine started in the background at ") || !strings.HasSuffix(string(body), " ===\nchild output\n") {
				t.Fatal("missing startup marker or child output")
			}
			rotated, err := os.ReadFile(path + ".1")
			if err != nil {
				t.Fatal(err)
			}
			if size > MaximumSize {
				if len(rotated) != size || strings.HasPrefix(string(body), "x") {
					t.Fatal("the old log did not replace the previous rotation")
				}
			} else if !strings.HasPrefix(string(body), strings.Repeat("x", size)) || string(rotated) != "previous rotation" {
				t.Fatal("rotated at the limit instead of appending")
			}
		})
	}
}

func TestTailReturnsOnlyTheLastLines(t *testing.T) {
	stateDir := t.TempDir()
	file, err := Open(stateDir, "test")
	if err != nil {
		t.Fatal(err)
	}
	for line := 0; line < TailLines+3; line++ {
		fmt.Fprintf(file, "line %d\n", line)
	}
	file.Close()
	tail, err := Tail(Path(stateDir))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(tail, "\n")
	if len(lines) != TailLines || lines[0] != "line 3" || lines[len(lines)-1] != fmt.Sprintf("line %d", TailLines+2) {
		t.Fatalf("tail = %q", tail)
	}
}

func TestTwoOpenWritersAlwaysAppend(t *testing.T) {
	stateDir := t.TempDir()
	first, err := Open(stateDir, "first")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(stateDir, "second")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	first.WriteString("first child\n")
	second.WriteString("second child\n")
	tail, err := Tail(Path(stateDir))
	if err != nil || !strings.HasSuffix(tail, "first child\nsecond child") || !strings.Contains(tail, "sshc first") || !strings.Contains(tail, "sshc second") {
		t.Fatalf("tail = %q, %v", tail, err)
	}
}
