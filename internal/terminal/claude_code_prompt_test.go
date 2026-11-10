package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/third_party/vt"
)

// claudeCodePromptScreens は、Claude Code 2.1.318 が tmux 3.6 の中で描いたものを持つ。エンジンのエミュレーターが
// tmux の出力を読んだ形で、ユーザー名とホスト名は置き換えてある。フォルダの名前が、その画面をどう読むべきかを
// 表す。
const claudeCodePromptScreens = "testdata/claude_code_prompts"

// screenWithText は、その大きさのエミュレーターが、text を1行ずつ出したあとに見せる画面である。
func screenWithText(text string, cols, rows int) [][]string {
	emulator := vt.NewSnapshotEmulator(cols, rows)
	_, _ = emulator.WriteString(strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "\r\n"))
	return emulator.ShownCells()
}

func TestEveryRecordedScreenReadsAsWhatItShows(t *testing.T) {
	kinds := map[string]ProgramStatusKind{"permission": ProgramStatusPermission, "question": ProgramStatusQuestion, "none": ""}
	for directory, want := range kinds {
		paths, err := filepath.Glob(filepath.Join(claudeCodePromptScreens, directory, "*.txt"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("no screens in %s (%v)", directory, err)
		}
		for _, path := range paths {
			t.Run(directory+"/"+filepath.Base(path), func(t *testing.T) {
				text, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var cols, rows int
				name := strings.TrimSuffix(filepath.Base(path), ".txt")
				if _, err := fmt.Sscanf(name[strings.LastIndexByte(name, '-')+1:], "%dx%d", &cols, &rows); err != nil {
					t.Fatalf("no size in the file name: %v", err)
				}
				got, asking := claudeCodePromptOnScreen(screenWithText(string(text), cols, rows))
				if got != want || asking != (want != "") {
					t.Fatalf("read as %q (asking %v), want %q on this screen:\n%s", got, asking, want, text)
				}
			})
		}
	}
}

func TestAQuestionIsReadInAnyPaneOfASplitWindow(t *testing.T) {
	question := []string{
		"Do you want to proceed?",
		" ❯ 1. Yes",
		"   2. No",
	}
	tests := map[string]string{
		"left":         "%-26s│other pane",
		"right":        "other pane          │%s",
		"middle":       "a │%-26s│ b",
		"heavy border": "other pane          ┃%s",
	}
	for name, layout := range tests {
		t.Run(name, func(t *testing.T) {
			var rows []string
			for _, row := range question {
				rows = append(rows, fmt.Sprintf(layout, row))
			}
			if kind, _ := claudeCodePromptOnScreen(screenWithText(strings.Join(rows, "\n"), 60, 5)); kind != ProgramStatusPermission {
				t.Fatalf("read as %q, want permission", kind)
			}
		})
	}
}

func TestAPaneBorderUnderTheOptionsEndsThePane(t *testing.T) {
	screen := screenWithText("Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n────────────\n$ ls", 40, 8)
	if kind, _ := claudeCodePromptOnScreen(screen); kind != ProgramStatusPermission {
		t.Fatalf("read as %q, want permission", kind)
	}
}

func TestAFooterFollowedByAShellPromptIsNotAQuestion(t *testing.T) {
	screen := screenWithText("❯ 1. A\n  2. B\n\nEnter to select · ↑/↓ to navigate · Esc to cancel\n$ ", 60, 8)
	if kind, asking := claudeCodePromptOnScreen(screen); asking {
		t.Fatalf("read as %q, want no question", kind)
	}
}

// 下に何も残さないプログラムが出した文字は、問いそのもののように読める。代替画面で残っている誤検出はこれで
// ある。画面の最後の行は tmux のステータス行として飛ばすが、tmux の外では、ページャー自身の最後の行が同じに
// 見える。
func TestTheWordsOfAQuestionWithNothingUnderThemReadAsAQuestion(t *testing.T) {
	tests := map[string]string{
		"watch":             "Every 2.0s: cat prompt.txt\n\nDo you want to proceed?\n ❯ 1. Yes\n   2. No",
		"less outside tmux": "Do you want to proceed?\n ❯ 1. Yes\n   2. No\n\n\n\n\nprompt.txt (END)",
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			if kind, _ := claudeCodePromptOnScreen(screenWithText(text, 40, 8)); kind != ProgramStatusPermission {
				t.Fatalf("read as %q, want permission", kind)
			}
		})
	}
}
