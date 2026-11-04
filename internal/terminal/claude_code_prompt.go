package terminal

import (
	"regexp"
	"slices"
	"strings"
)

// claudeCodePromptShape is one kind of question Claude Code asks, as it reads on screen around the option the person
// has selected. A shape looks at the words above the options or at the footer below them.
type claudeCodePromptShape struct {
	// kind is what Claude Code itself reports for this question over OSC 7501.
	kind ProgramStatusKind
	// above is words in the rows above the selected option, up to the rule drawn over the question. The rows are
	// joined, since a question folds wherever the pane is narrow.
	above string
	// footer matches the rows of the footer, joined the same way.
	footer *regexp.Regexp
}

// claudeCodePromptShapes are the questions the engine can read, as Claude Code 2.1.318 draws them. The screens they
// were taken from are in testdata/claude_code_prompts. A later version that rewords one is no longer read, so it goes
// unnoticed instead of being reported wrongly: add its screen there and its words here.
var claudeCodePromptShapes = []claudeCodePromptShape{
	// AskUserQuestion. The question and its options come from the model in any language, so this comes first: only the
	// footer is Claude Code's. Without "to navigate" it would also read Claude Code's own menus, such as /theme, which
	// the person opened.
	{kind: ProgramStatusQuestion, footer: regexp.MustCompile(`^Enter to select .*to navigate .*Esc to cancel$`)},
	// AskUserQuestion's last tab when it asks several questions.
	{kind: ProgramStatusQuestion, above: "Ready to submit your answers?"},
	// Bash, Write, Edit and WebFetch: "Do you want to proceed?", "Do you want to create hello.txt?".
	{kind: ProgramStatusPermission, above: "Do you want"},
	// ExitPlanMode with a plan, and with none.
	{kind: ProgramStatusPermission, above: "Would you like to proceed?"},
	{kind: ProgramStatusPermission, above: "Exit plan mode?"},
}

// claudeCodeSelectionMarks are the marks by the option a question has selected. tmux draws "_" for "❯" when the server's
// locale is not UTF-8.
var claudeCodeSelectionMarks = map[string]bool{"❯": true, "_": true}

// claudeCodeOptionNumber follows a selection mark. It tells an option from the input row, which has the mark too.
var claudeCodeOptionNumber = regexp.MustCompile(`^ \d+\. `)

// claudeCodeInputMark starts the row Claude Code takes the person's next message on. That row is not shown while
// Claude Code asks a question.
const claudeCodeInputMark = "❯"

// tmuxPaneBorders are the cells of a vertical pane border, with its junctions: single (tmux's default), heavy and
// double pane-border-lines.
var tmuxPaneBorders = map[string]bool{
	"│": true, "├": true, "┤": true, "┼": true,
	"┃": true, "┣": true, "┫": true, "╋": true,
	"║": true, "╠": true, "╣": true, "╬": true,
}

// claudeCodePromptOnScreen returns what a Claude Code question on screen waits for, and whether one is there. screen
// is the text of each cell. Any pane of a split window may hold the question.
func claudeCodePromptOnScreen(screen [][]string) (ProgramStatusKind, bool) {
	for rowIndex, row := range screen {
		for column, cell := range row {
			if !claudeCodeSelectionMarks[cell] {
				continue
			}
			left := tmuxPaneEdge(row, column, -1)
			if !isBlank(row[left+1:column]) || !claudeCodeOptionNumber.MatchString(strings.Join(row[column+1:], "")) {
				continue
			}
			if kind, asking := newPromptPane(screen, rowIndex, left, tmuxPaneEdge(row, column, 1)).question(); asking {
				return kind, true
			}
		}
	}
	return "", false
}

// tmuxPaneEdge returns the column of the nearest pane border from column in the direction step (-1 or 1), or the
// column just outside the row.
func tmuxPaneEdge(row []string, column, step int) int {
	for column += step; column >= 0 && column < len(row); column += step {
		if tmuxPaneBorders[row[column]] {
			return column
		}
	}
	return column
}

func isBlank(cells []string) bool {
	for _, cell := range cells {
		if cell != " " {
			return false
		}
	}
	return true
}

// promptPane is the text of one pane's rows around a selected option. A pane that takes the whole width is not told
// from one above or below it until a rule ends it.
type promptPane struct {
	// above holds the rows above the selected option, nearest first; below the rows under it, nearest first.
	above, below []string
	// reachesScreenBottom says that the last of below is the screen's last row, where tmux keeps its status line.
	reachesScreenBottom bool
}

func newPromptPane(screen [][]string, selectedRow, left, right int) promptPane {
	inPane := func(row []string) bool {
		return (left < 0 || tmuxPaneBorders[row[left]]) && (right >= len(row) || tmuxPaneBorders[row[right]])
	}
	text := func(row []string) string {
		return strings.TrimRight(strings.Join(row[left+1:right], ""), " ")
	}
	var pane promptPane
	for index := selectedRow - 1; index >= 0 && inPane(screen[index]); index-- {
		pane.above = append(pane.above, text(screen[index]))
	}
	for index := selectedRow + 1; index < len(screen) && inPane(screen[index]); index++ {
		pane.below = append(pane.below, text(screen[index]))
		pane.reachesScreenBottom = index == len(screen)-1
	}
	return pane
}

// question returns the kind of the shape the pane reads as.
func (p promptPane) question() (ProgramStatusKind, bool) {
	for _, shape := range claudeCodePromptShapes {
		switch {
		case shape.footer != nil && p.endsWithFooter(shape.footer):
			return shape.kind, true
		case shape.above != "" && strings.Contains(p.textAboveOptions(), shape.above) && p.endsAfter(0):
			return shape.kind, true
		}
	}
	return "", false
}

// textAboveOptions is the rows from the rule over the question down to the selected option, as one line.
func (p promptPane) textAboveOptions() string {
	var rows []string
	for _, row := range p.above {
		if isRuleRow(row) {
			break
		}
		rows = append(rows, strings.TrimSpace(row))
	}
	slices.Reverse(rows)
	return strings.Join(rows, " ")
}

// endsWithFooter reports whether a footer stands under the options, folded over any number of rows, with nothing
// else after it.
func (p promptPane) endsWithFooter(footer *regexp.Regexp) bool {
	for start := range p.below {
		text := ""
		for index := start; index < len(p.below) && p.below[index] != ""; index++ {
			text = strings.TrimSpace(text + " " + strings.TrimSpace(p.below[index]))
			if footer.MatchString(text) {
				return p.endsAfter(index + 1)
			}
		}
	}
	return false
}

// endsAfter reports whether the rows of below from index on can belong to a question that is still being asked. This
// is what tells the question from the same words printed by `cat` or quoted in Claude Code's own answer: Claude Code
// indents what follows its options, and shows no input row, while a shell prompt under the words starts at the edge.
func (p promptPane) endsAfter(index int) bool {
	for ; index < len(p.below); index++ {
		row := p.below[index]
		switch {
		case p.reachesScreenBottom && index == len(p.below)-1:
			return true
		case isRuleRow(row):
			return index+1 == len(p.below) || !strings.HasPrefix(p.below[index+1], claudeCodeInputMark)
		case row != "" && row[0] != ' ':
			return false
		}
	}
	return true
}

// isRuleRow reports whether a row is a line across the pane: one Claude Code draws, or the border between two panes.
func isRuleRow(row string) bool {
	row = strings.TrimLeft(row, " ")
	return strings.HasPrefix(row, "─") || strings.HasPrefix(row, "╌")
}
