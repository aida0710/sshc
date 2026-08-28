// Package effective は、OpenSSH がひとつの Host alias に対して実際に使う設定を
// 説明し、評価する。
//
// このパッケージのどれも、ユーザーから得た明示的な確認を呼び出し側が渡さない限り、
// プログラムを起動しない。OpenSSH の設定を評価すること自体が、コマンドを実行しうる
// からである。
package effective

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"sshc/internal/config"
)

// TokenEscapeWarning は、実行を伴うすべてのディレクティブの隣に表示される。
//
// OpenSSH は %h、%p、%r などを、シェル向けの引用をせずに実行するコマンドへ展開する。
// そのため、設定から取られたホスト名やユーザーの値が、そのままそのシェルへ届き
// うる。
const TokenEscapeWarning = "OpenSSH does not shell-escape the tokens it expands. A hostname, port or user value can reach the shell of this command unchanged."

// Executable は、OpenSSH にプログラムを実行させうるディレクティブひとつ。
type Executable struct {
	// Keyword は正規の表記。Match の criterion の場合は "Match exec"。
	Keyword string
	// Command は、ファイルに現れるとおりの引数テキスト。
	Command string
	Path    string
	// Line は 1 始まり。
	Line int
	// Condition は、それを囲む Host または Match のヘッダー。グローバルブロックでは空。
	Condition string
	// OnEvaluate は、設定を評価するだけでそれが実行される場合に true。
	OnEvaluate bool
	// OnConnect は、接続を確立するとそれが実行される場合に true。
	OnConnect bool
	// Overridable は、コマンドラインオプションで一回分だけ無効にできる場合に true。
	Overridable bool
}

// Report は、設定から到達できる実行を伴うディレクティブを列挙する。
type Report struct {
	Directives []Executable
}

var executableDirectives = map[string]Executable{
	"proxycommand":      {Keyword: "ProxyCommand", OnConnect: true},
	"knownhostscommand": {Keyword: "KnownHostsCommand", OnConnect: true},
	"localcommand":      {Keyword: "LocalCommand", OnConnect: true, Overridable: true},
	"remotecommand":     {Keyword: "RemoteCommand", OnConnect: true, Overridable: true},
}

// Scan は、グラフ内のすべてのファイルから実行を伴うディレクティブを集める。
func Scan(graph *config.Graph) Report {
	return scanAll(graph)
}

// ScanForAlias は、その alias の接続時に適用されうる実行可能ディレクティブを集める。
// 別の Host ブロックにだけある ProxyCommand などを警告へ混ぜない。一方、Match exec
// は設定を読む過程で評価されうるため、Match ブロックは保守的に残す。
func ScanForAlias(graph *config.Graph, alias string) Report {
	report := Report{}
	if graph == nil {
		return report
	}
	seen := make(map[string]bool)
	scanAliasFile(graph, graph.Root, alias, true, "", map[string]bool{}, seen, &report)
	return report
}

func scanAll(graph *config.Graph) Report {
	report := Report{}
	if graph == nil {
		return report
	}
	for _, filePath := range graph.Order {
		node := graph.Nodes[filePath]
		if node == nil || node.File == nil {
			continue
		}
		for _, block := range node.File.Blocks() {
			condition := node.File.Condition(block)
			if block.Kind == config.BlockMatch {
				for _, criterion := range block.Criteria {
					// config は Match の criterion キーワードを小文字にする。
					if criterion.Keyword != "exec" {
						continue
					}
					report.Directives = append(report.Directives, Executable{
						Keyword:    "Match exec",
						Command:    criterion.Argument,
						Path:       filePath,
						Line:       block.Header + 1,
						Condition:  condition,
						OnEvaluate: true,
						OnConnect:  true,
					})
				}
			}
			for index := block.Start; index < block.End; index++ {
				line := node.File.Lines[index]
				if line.Kind != config.LineDirective {
					continue
				}
				template, ok := executableDirectives[strings.ToLower(line.Keyword)]
				if !ok {
					continue
				}
				directive := template
				directive.Command = argumentText(line)
				directive.Path = filePath
				directive.Line = index + 1
				directive.Condition = condition
				report.Directives = append(report.Directives, directive)
			}
		}
	}
	return report
}

// scanAliasFile は Include を、その行が置かれた Host / Match の適用状態ごと
// 引き継いで読む。ファイル先頭の「グローバル」ブロックは、Include 元では実際には
// 独立したグローバル設定ではない。たとえば Host pcluster-head の中から読み込まれた
// ファイルの先頭に ProxyCommand があれば、そのコマンドは pcluster-head にだけ
// 適用される。この状態を渡さず graph.Order を走査すると、別ホストの警告へ漏れる。
func scanAliasFile(
	graph *config.Graph,
	filePath string,
	alias string,
	inheritedApplies bool,
	inheritedCondition string,
	chain map[string]bool,
	seen map[string]bool,
	report *Report,
) {
	node := graph.Nodes[filePath]
	if node == nil || node.File == nil || chain[filePath] {
		return
	}
	chain[filePath] = true
	defer delete(chain, filePath)

	blocks := node.File.Blocks()
	position := 0
	applies := inheritedApplies
	condition := inheritedCondition

	for index, line := range node.File.Lines {
		if position+1 < len(blocks) && blocks[position+1].Header == index {
			position++
			block := blocks[position]
			condition = node.File.Condition(block)
			switch block.Kind {
			case config.BlockHost:
				_, applies = blockApplies(block, alias)
			case config.BlockMatch:
				// Match は接続時の user / address や exec に依存しうる。ここで
				// 実行して判定せず、到達した Match は保守的に警告へ残す。
				applies = true
				for _, criterion := range block.Criteria {
					if criterion.Keyword != "exec" {
						continue
					}
					appendExecutable(report, seen, Executable{
						Keyword: "Match exec", Command: criterion.Argument,
						Path: filePath, Line: block.Header + 1, Condition: condition,
						OnEvaluate: true, OnConnect: true,
					})
				}
			}
			continue
		}
		if line.Kind != config.LineDirective || !applies {
			continue
		}
		if config.EqualKeyword(line.Keyword, "Include") {
			for _, edge := range node.Includes {
				if edge.Line != index+1 {
					continue
				}
				for _, match := range edge.Matches {
					scanAliasFile(graph, match, alias, applies, condition, chain, seen, report)
				}
			}
			continue
		}
		template, ok := executableDirectives[strings.ToLower(line.Keyword)]
		if !ok {
			continue
		}
		directive := template
		directive.Command = argumentText(line)
		directive.Path = filePath
		directive.Line = index + 1
		directive.Condition = condition
		appendExecutable(report, seen, directive)
	}
}

func appendExecutable(report *Report, seen map[string]bool, directive Executable) {
	key := fmt.Sprintf("%s\x00%d\x00%s\x00%s", directive.Path, directive.Line, directive.Keyword, directive.Command)
	if seen[key] {
		return
	}
	seen[key] = true
	report.Directives = append(report.Directives, directive)
}

// Unavoidable は、どのコマンドラインオプションでも無効にできないディレクティブを
// 返す。そのいずれかを実行することになる接続は、ユーザーが正確なコマンドテキストを
// 確認したあとでのみ開始される。
func (r Report) Unavoidable() []Executable {
	var remaining []Executable
	for _, directive := range r.Directives {
		if directive.OnConnect && !directive.Overridable {
			remaining = append(remaining, directive)
		}
	}
	return remaining
}

// Evidence は、確認ダイアログが表示しなければならない内容の安定したダイジェスト。
//
// アクショントークンはこの値に結び付けられるので、確認と実行のあいだに編集された
// 設定は、暗黙に別のコマンドを実行するのではなく、その確認を無効に
// する。
func (r Report) Evidence() string {
	entries := make([]string, 0, len(r.Directives))
	for _, directive := range r.Directives {
		entries = append(entries, fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s",
			directive.Keyword, directive.Command, directive.Path, directive.Line, directive.Condition))
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

// argumentText は、ディレクティブの引数部分を、インデント・キーワード・区切り・
// 行末を除いて、書かれたとおりに返す。
func argumentText(line config.Line) string {
	var builder strings.Builder
	for _, argument := range line.Arguments {
		builder.WriteString(argument.Lead)
		builder.WriteString(argument.Raw)
	}
	return strings.TrimSpace(builder.String())
}
