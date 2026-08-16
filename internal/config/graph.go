package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"

	"sshc/internal/platform/nativepath"
)

// Severity は、表示のために診断を順位付けする。エンジンが診断を黙った修復に
// 変えることは決してない。
type Severity uint8

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// 診断コードは、UI が自前の文言に対応付ける安定した識別子。
const (
	DiagnosticIncludeNoMatch       = "include_no_match"
	DiagnosticIncludeUnreadable    = "include_unreadable"
	DiagnosticIncludeCycle         = "include_cycle"
	DiagnosticIncludeDuplicate     = "include_duplicate"
	DiagnosticIncludeConditional   = "include_conditional"
	DiagnosticIncludeOutsideRoot   = "include_outside_root"
	DiagnosticIncludeDepthExceeded = "include_depth_exceeded"
	DiagnosticIncludeUnsupported   = "include_unsupported_expansion"
	DiagnosticIncludeEmpty         = "include_without_argument"
	DiagnosticUnstructuredLine     = "unstructured_line"
)

// ErrPathNotAbsolute は、絶対パスでないパスからグラフの走査を始めるよう求められた
// ときに返る。
var ErrPathNotAbsolute = errors.New("configuration path must be absolute")

// Diagnostic は、ユーザーが判断すべき事柄を記述する。Line は 1 始まりで、ファイル
// 全体に関する診断のときは 0 になる。
type Diagnostic struct {
	Severity Severity
	Code     string
	Path     string
	Line     int
	Detail   string
}

// Edge は、Include の引数ひとつと、それが解決した先のファイル群。
type Edge struct {
	FromPath  string
	Line      int
	Pattern   string
	Expanded  string
	Matches   []string
	Condition string
}

// Node は、グラフ内の設定ファイルひとつ。
type Node struct {
	Path     string
	Editable bool
	Missing  bool
	File     *File
	Includes []Edge
	Loads    int
}

// Graph は、ひとつのエントリファイルから到達できる Include 構造。
//
// ルートより下の節点は、Root 自身の綴りで鍵付けされる。同じファイルを二通りに
// 綴った Include は、ひとつの節点に落ち着く。ルートの外の節点は、Include が
// 名指したままの綴りを保つ——よそのファイルの綴りをこちらで決める理由が無い。
type Graph struct {
	Root        string
	Order       []string
	Nodes       map[string]*Node
	Diagnostics []Diagnostic
	// identities は、同じファイルの別の綴りを最初の節点へ結び直す。Windows の
	// 大小文字違いは同じファイルなので、これが無いと同じ内容が二度読み込まれ、
	// 循環はいつまでも見つからない。
	identities map[string]*Node
}

// Resolve は、エントリファイルとそれが include するすべてのファイルを読む。
// Resolve がエラーを返すのは、リクエスト自体が不正なときだけである。読めない
// ファイル、循環、非対応のパターンは診断として報告されるので、UI は失敗する
// 代わりに実際の構造を表示できる。
func (r Resolver) Resolve(rootPath string) (*Graph, error) {
	if !nativepath.Supported(rootPath) {
		return nil, ErrPathNotAbsolute
	}
	graph := &Graph{
		Root:       filepath.Clean(rootPath),
		Nodes:      make(map[string]*Node),
		identities: make(map[string]*Node),
	}
	r.walk(graph, graph.Root, nil, 0)
	return graph, nil
}

func (g *Graph) diagnose(severity Severity, code, filePath string, line int, detail string) {
	g.Diagnostics = append(g.Diagnostics, Diagnostic{
		Severity: severity,
		Code:     code,
		Path:     filePath,
		Line:     line,
		Detail:   detail,
	})
}

func (r Resolver) insideRoot(candidate string) bool {
	return nativepath.Contains(r.Root, candidate)
}

// canonical は、ルートより下のパスをルート自身の綴りに揃える。
//
// **これが無いと、層によって同じファイルが別の名前になる。** Windows で
// `C:/USERS/A/.ssh/conf.d/x.conf` を include すると、その節点の鍵は打たれた
// 綴りのまま残る。application 層はルートから組み立てた綴りで引くので、一覧に
// 出て編集可能と表示されたホストが、開こうとした瞬間に見つからないと言われる。
//
// ルートの外はそのまま返す。よそのファイルの綴りをこちらで決める理由が無い。
func (r Resolver) canonical(candidate string) string {
	cleaned := filepath.Clean(candidate)
	if !nativepath.Contains(r.Root, cleaned) {
		return cleaned
	}
	relative, err := filepath.Rel(r.Root, cleaned)
	if err != nil {
		return cleaned
	}
	if relative == "." {
		return filepath.Clean(r.Root)
	}
	return filepath.Join(r.Root, relative)
}

func (r Resolver) walk(graph *Graph, filePath string, chain []string, depth int) {
	node := &Node{Path: filePath, Editable: r.insideRoot(filePath), Loads: 1}
	graph.Nodes[filePath] = node
	graph.identities[nativepath.Identity(filePath)] = node
	graph.Order = append(graph.Order, filePath)

	contents, err := r.Loader.ReadFile(filePath)
	if err != nil {
		node.Missing = errors.Is(err, fs.ErrNotExist)
		graph.diagnose(SeverityError, DiagnosticIncludeUnreadable, filePath, 0, err.Error())
		return
	}
	node.File = Parse(contents)

	currentChain := make([]string, 0, len(chain)+1)
	currentChain = append(currentChain, chain...)
	currentChain = append(currentChain, nativepath.Identity(filePath))

	generatedStart, generatedEnd, generated := r.generatedLines(node.File)

	for index, line := range node.File.Lines {
		lineNumber := index + 1
		if line.Kind == LineUnstructured {
			graph.diagnose(SeverityInfo, DiagnosticUnstructuredLine, filePath, lineNumber, "line is preserved verbatim and can only be edited as raw text")
			continue
		}
		if line.Kind != LineDirective || !EqualKeyword(line.Keyword, "Include") {
			continue
		}

		condition := node.File.Condition(node.File.BlockAt(index))
		if condition != "" {
			graph.diagnose(SeverityWarning, DiagnosticIncludeConditional, filePath, lineNumber, condition)
		}
		values := line.Values()
		if len(values) == 0 {
			graph.diagnose(SeverityWarning, DiagnosticIncludeEmpty, filePath, lineNumber, "")
			continue
		}

		for _, value := range values {
			edge := Edge{FromPath: filePath, Line: lineNumber, Pattern: value, Condition: condition}
			expanded, expandErr := r.expandPattern(value)
			if expandErr != nil {
				graph.diagnose(SeverityWarning, DiagnosticIncludeUnsupported, filePath, lineNumber, value)
				node.Includes = append(node.Includes, edge)
				continue
			}
			edge.Expanded = expanded

			matches, globErr := r.Loader.Glob(expanded)
			if globErr != nil {
				graph.diagnose(SeverityWarning, DiagnosticIncludeUnreadable, filePath, lineNumber, globErr.Error())
				node.Includes = append(node.Includes, edge)
				continue
			}
			// **綴りを揃えるのは、辺に載せる前である。** 節点の鍵だけを揃えて辺に
			// 生の綴りを残すと、同じファイルが二つの名前で現れ、辺をたどる側——
			// ディレクティブの走査、実効設定、スナップショット、Include 行の
			// 書き換え——がその節点を見つけられなくなる。
			for index, match := range matches {
				matches[index] = r.canonical(match)
			}
			sort.Strings(matches)
			// 生成領域の内側は黙る。その行を書いたのはこのアプリケーション自身で、
			// 宣言されたグループがまだ空であることは正常な状態である。外側は人が
			// 書いた行なので、何にも一致しないのは打ち間違いかもしれない。
			insideGenerated := generated && index > generatedStart && index < generatedEnd
			if len(matches) == 0 && !insideGenerated {
				graph.diagnose(SeverityWarning, DiagnosticIncludeNoMatch, filePath, lineNumber, expanded)
			}
			edge.Matches = matches
			node.Includes = append(node.Includes, edge)

			for _, match := range matches {
				if !r.insideRoot(match) {
					graph.diagnose(SeverityInfo, DiagnosticIncludeOutsideRoot, filePath, lineNumber, match)
				}
				identity := nativepath.Identity(match)
				if slicesContains(currentChain, identity) {
					graph.diagnose(SeverityError, DiagnosticIncludeCycle, filePath, lineNumber, match)
					continue
				}
				if existing, seen := graph.identities[identity]; seen {
					existing.Loads++
					graph.diagnose(SeverityWarning, DiagnosticIncludeDuplicate, filePath, lineNumber, match)
					continue
				}
				if depth+1 > r.maxDepth() {
					graph.diagnose(SeverityError, DiagnosticIncludeDepthExceeded, filePath, lineNumber, match)
					continue
				}
				r.walk(graph, match, currentChain, depth+1)
			}
		}
	}
}

func slicesContains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
