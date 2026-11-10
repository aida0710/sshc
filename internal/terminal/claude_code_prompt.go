package terminal

import (
	"regexp"
	"slices"
	"strings"
)

// claudeCodePromptShape は、Claude Code が尋ねる問いの1つの種類である。利用者が選んでいる選択肢のまわりに、
// 画面でどう読めるかで表す。選択肢の上の言葉か、下のフッターを見る。
type claudeCodePromptShape struct {
	// kind は、この問いについて Claude Code 自身が OSC 7501 で報告する種類である。
	kind ProgramStatusKind
	// above は、選んでいる選択肢より上の行にある言葉である。問いの上に引かれた線までを見る。問いはペインの狭い
	// ところで折り返すので、行をつないで見る。
	above string
	// footer は、同じようにつないだフッターの行に合う。
	footer *regexp.Regexp
}

// claudeCodePromptShapes は、エンジンが読める問いである。Claude Code 2.1.318 の描き方による。もとにした画面は
// testdata/claude_code_prompts にある。あとの版が言い回しを変えた問いは読めなくなるので、誤って報告される
// のではなく、気づかれないままになる。そのときは、画面をそこに足し、言葉をここに足す。
var claudeCodePromptShapes = []claudeCodePromptShape{
	// AskUserQuestion。問いと選択肢は、モデルがどの言語でも書くので、これを最初に置く。Claude Code のものは
	// フッターだけである。"to navigate" が無いと、利用者が開いた /theme のような Claude Code 自身のメニューも
	// 読んでしまう。
	{kind: ProgramStatusQuestion, footer: regexp.MustCompile(`^Enter to select .*to navigate .*Esc to cancel$`)},
	// AskUserQuestion がいくつも尋ねるときの、最後のタブ。
	{kind: ProgramStatusQuestion, above: "Ready to submit your answers?"},
	// Bash、Write、Edit、WebFetch。"Do you want to proceed?"、"Do you want to create hello.txt?"。
	{kind: ProgramStatusPermission, above: "Do you want"},
	// ExitPlanMode。計画があるときと、無いとき。
	{kind: ProgramStatusPermission, above: "Would you like to proceed?"},
	{kind: ProgramStatusPermission, above: "Exit plan mode?"},
}

// claudeCodeSelectionMarks は、問いが選んでいる選択肢の横の印である。サーバーのロケールが UTF-8 でないとき、
// tmux は "❯" を "_" と描く。
var claudeCodeSelectionMarks = map[string]bool{"❯": true, "_": true}

// claudeCodeOptionNumber は、選択の印に続く。同じ印を持つ入力の行と、選択肢を見分ける。
var claudeCodeOptionNumber = regexp.MustCompile(`^ \d+\. `)

// claudeCodeInputMark は、Claude Code が利用者の次のメッセージを受け取る行の先頭である。Claude Code が
// 問いを尋ねているあいだ、その行は出ない。
const claudeCodeInputMark = "❯"

// tmuxPaneBorders は、ペインの縦の境と、その交点のマスである。pane-border-lines の single（tmux の既定）、
// heavy、double を含む。
var tmuxPaneBorders = map[string]bool{
	"│": true, "├": true, "┤": true, "┼": true,
	"┃": true, "┣": true, "┫": true, "╋": true,
	"║": true, "╠": true, "╣": true, "╬": true,
}

// claudeCodePromptOnScreen は、画面にある Claude Code の問いが待っているものと、問いがあるかを返す。screen は
// 各マスの文字である。分割したウィンドウのどのペインに問いがあってもよい。
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

// tmuxPaneEdge は、column から step（-1 か 1）の向きで、いちばん近いペインの境の列を返す。無ければ、行の
// すぐ外の列を返す。
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

// promptPane は、選んでいる選択肢のまわりの、1つのペインの行の文字である。幅いっぱいのペインは、線で
// 終わるまで、上や下のペインと見分けられない。
type promptPane struct {
	// above は、選んでいる選択肢より上の行を、近い順に持つ。below は、下の行を、近い順に持つ。
	above, below []string
	// reachesScreenBottom は、below の最後が画面の最後の行であることを表す。tmux はそこにステータス行を置く。
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

// question は、このペインが読める形の種類を返す。
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

// textAboveOptions は、問いの上の線から、選んでいる選択肢までの行を、1行にしたものである。
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

// endsWithFooter は、選択肢の下にフッターがあり、そのあとに何も無いかを返す。フッターは何行に折り返して
// いてもよい。
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

// endsAfter は、below の index 以降の行が、まだ尋ねている問いのものでありうるかを返す。これで、問いを、`cat` が
// 出した同じ言葉や、Claude Code 自身の答えに引用された同じ言葉と見分ける。Claude Code は選択肢のあとを
// 字下げし、入力の行を出さない。言葉の下のシェルのプロンプトは、端から始まる。
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

// isRuleRow は、行がペインを横切る線かを返す。Claude Code が描く線か、2つのペインの境である。
func isRuleRow(row string) bool {
	row = strings.TrimLeft(row, " ")
	return strings.HasPrefix(row, "─") || strings.HasPrefix(row, "╌")
}
