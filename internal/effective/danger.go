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
//
// 読み込み順と Include の扱いは Resolve と同じ walkLoadOrder に従う。別の Host
// ブロックにだけある ProxyCommand などを警告へ混ぜず、Resolve が接続に使う
// ProxyCommand は必ず挙げる。Host pcluster-head の中から読み込まれたファイルの先頭に
// ある ProxyCommand は、pcluster-head にだけ適用される。
//
// Match は接続時の user やアドレスや exec の結果に依存しうる。ここで実行して判定
// せず、到達した Match は効くものとして保守的に扱う。Match exec は、一致していない
// ブロックの中の Include から読まれたものでも OpenSSH が実行するので、必ず挙げる。
func ScanForAlias(graph *config.Graph, alias string) Report {
	report := Report{}
	if graph == nil {
		return report
	}
	seen := make(map[string]bool)
	walkLoadOrder(graph, loadOrderVisitor{
		enterBlock: func(header blockHeader) (string, bool) {
			if header.block.Kind != config.BlockMatch {
				return hostBlockApplies(header.block, alias)
			}
			for _, execution := range matchExecutions(header) {
				appendExecutable(&report, seen, execution)
			}
			return "", true
		},
		directive: func(found directiveLine) {
			if directive, ok := executableAt(found); ok {
				appendExecutable(&report, seen, directive)
			}
		},
	})
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
			header := blockHeader{path: filePath, block: block, condition: node.File.Condition(block)}
			if block.Kind == config.BlockMatch {
				report.Directives = append(report.Directives, matchExecutions(header)...)
			}
			for index := block.Start; index < block.End; index++ {
				found := directiveLine{
					path: filePath, number: index + 1, line: node.File.Lines[index],
					state: blockState{condition: header.condition},
				}
				if directive, ok := executableAt(found); ok {
					report.Directives = append(report.Directives, directive)
				}
			}
		}
	}
	return report
}

// matchExecutions は、Match の見出しにある exec の条件をそれぞれ Executable にする。
func matchExecutions(header blockHeader) []Executable {
	var executions []Executable
	for _, criterion := range header.block.Criteria {
		// config は Match の criterion キーワードを小文字にする。
		if criterion.Keyword != "exec" {
			continue
		}
		executions = append(executions, Executable{
			Keyword: "Match exec", Command: criterion.Argument,
			Path: header.path, Line: header.number(), Condition: header.condition,
			OnEvaluate: true, OnConnect: true,
		})
	}
	return executions
}

// executableAt は、行がプログラムを実行させうるディレクティブなら、その Executable を返す。
func executableAt(found directiveLine) (Executable, bool) {
	if found.line.Kind != config.LineDirective {
		return Executable{}, false
	}
	template, ok := executableDirectives[strings.ToLower(found.line.Keyword)]
	if !ok {
		return Executable{}, false
	}
	directive := template
	directive.Command = strings.Join(directiveValues(found.line), " ")
	directive.Path = found.path
	directive.Line = found.number
	directive.Condition = found.state.condition
	return directive, true
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
