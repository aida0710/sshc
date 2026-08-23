package config

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

type fakeLoader struct {
	files map[string]string
	fail  map[string]error
	// globs は、パターンごとの一致をそのまま与える。filepath.Match は大小文字を
	// 区別するので、Windows の別表記が返ってくる場面はこれでしか作れない。
	globs map[string][]string
}

func (l fakeLoader) ReadFile(name string) ([]byte, error) {
	if err, ok := l.fail[name]; ok {
		return nil, err
	}
	contents, ok := l.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(contents), nil
}

func (l fakeLoader) Glob(pattern string) ([]string, error) {
	if matches, ok := l.globs[pattern]; ok {
		return matches, nil
	}
	var matches []string
	for name := range l.files {
		if matched, err := filepath.Match(pattern, name); err == nil && matched {
			matches = append(matches, name)
		}
	}
	for name := range l.fail {
		if matched, err := filepath.Match(pattern, name); err == nil && matched {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

func resolverFor(files map[string]string) Resolver {
	resolver := newTestResolver()
	resolver.Loader = fakeLoader{files: files}
	return resolver
}

func diagnosticCodes(graph *Graph) []string {
	codes := make([]string, 0, len(graph.Diagnostics))
	for _, diagnostic := range graph.Diagnostics {
		codes = append(codes, diagnostic.Code)
	}
	return codes
}

func requireDiagnostic(t *testing.T, graph *Graph, code string) Diagnostic {
	t.Helper()
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == code {
			return diagnostic
		}
	}
	t.Fatalf("diagnostic %q missing, got %v", code, diagnosticCodes(graph))
	return Diagnostic{}
}

func TestResolveLoadsIncludedFilesInLexicalGlobOrder(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig: "Include conf.d/*.conf\nHost direct\n",
		filepath.Join(testRoot, "conf.d", "20-b.conf"): "Host bravo\n",
		filepath.Join(testRoot, "conf.d", "10-a.conf"): "Host alpha\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		testConfig,
		filepath.Join(testRoot, "conf.d", "10-a.conf"),
		filepath.Join(testRoot, "conf.d", "20-b.conf"),
	}
	if len(graph.Order) != len(want) {
		t.Fatalf("order = %#v", graph.Order)
	}
	for index := range want {
		if graph.Order[index] != want[index] {
			t.Fatalf("order[%d] = %q, want %q", index, graph.Order[index], want[index])
		}
	}
	root := graph.Nodes[testConfig]
	if !root.Editable || root.Missing || root.File == nil {
		t.Fatalf("root node = %#v", root)
	}
	if len(root.Includes) != 1 || root.Includes[0].Line != 1 || len(root.Includes[0].Matches) != 2 {
		t.Fatalf("root includes = %#v", root.Includes)
	}
	if root.Includes[0].Condition != "" {
		t.Errorf("top-level include has condition %q", root.Includes[0].Condition)
	}
}

func TestResolveStopsAtIncludeCycle(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig:                        "Include a.conf\n",
		filepath.Join(testRoot, "a.conf"): "Include b.conf\n",
		filepath.Join(testRoot, "b.conf"): "Include config\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	cycle := requireDiagnostic(t, graph, DiagnosticIncludeCycle)
	if cycle.Path != filepath.Join(testRoot, "b.conf") || cycle.Severity != SeverityError {
		t.Fatalf("cycle diagnostic = %#v", cycle)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(graph.Nodes))
	}
}

func TestResolveCountsDuplicateIncludesWithoutWalkingTwice(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig:                             "Include shared.conf\nInclude shared.conf\n",
		filepath.Join(testRoot, "shared.conf"): "Host shared\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	requireDiagnostic(t, graph, DiagnosticIncludeDuplicate)
	if loads := graph.Nodes[filepath.Join(testRoot, "shared.conf")].Loads; loads != 2 {
		t.Fatalf("loads = %d, want 2", loads)
	}
	if len(graph.Order) != 2 {
		t.Fatalf("order = %#v", graph.Order)
	}
}

func TestResolveFlagsConditionalIncludes(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig:                           "Host work\n\tInclude work.conf\n",
		filepath.Join(testRoot, "work.conf"): "User ops\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	conditional := requireDiagnostic(t, graph, DiagnosticIncludeConditional)
	if conditional.Line != 2 {
		t.Fatalf("conditional diagnostic = %#v", conditional)
	}
	edge := graph.Nodes[testConfig].Includes[0]
	if edge.Condition != "Host work" {
		t.Fatalf("edge condition = %q", edge.Condition)
	}
}

func TestResolveMarksFilesOutsideTheRootAsNotEditable(t *testing.T) {
	shared := filepath.Join(testOutside, "shared.conf")
	graph, err := resolverFor(map[string]string{
		testConfig: "Include " + filepath.ToSlash(shared) + "\n",
		shared:     "Host shared\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	requireDiagnostic(t, graph, DiagnosticIncludeOutsideRoot)
	outside := graph.Nodes[shared]
	if outside == nil || outside.Editable || outside.File == nil {
		t.Fatalf("outside node = %#v", outside)
	}
}

func TestResolveReportsPatternsItRefusesToExpand(t *testing.T) {
	graph, err := resolverFor(map[string]string{
		testConfig: "Include %h/config\nInclude missing/*.conf\nInclude\n",
	}).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	unsupported := requireDiagnostic(t, graph, DiagnosticIncludeUnsupported)
	if unsupported.Line != 1 || unsupported.Severity != SeverityWarning {
		t.Fatalf("unsupported diagnostic = %#v", unsupported)
	}
	requireDiagnostic(t, graph, DiagnosticIncludeNoMatch)
	requireDiagnostic(t, graph, DiagnosticIncludeEmpty)
	edges := graph.Nodes[testConfig].Includes
	if len(edges) != 2 || edges[0].Expanded != "" || len(edges[0].Matches) != 0 {
		t.Fatalf("edges = %#v", edges)
	}
}

func TestResolveReportsUnreadableAndMissingRoot(t *testing.T) {
	broken := filepath.Join(testRoot, "broken.conf")
	resolver := newTestResolver()
	resolver.Loader = fakeLoader{
		files: map[string]string{testConfig: "Include broken.conf\n"},
		fail:  map[string]error{broken: fs.ErrPermission},
	}
	graph, err := resolver.Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	unreadable := requireDiagnostic(t, graph, DiagnosticIncludeUnreadable)
	if unreadable.Severity != SeverityError {
		t.Fatalf("unreadable diagnostic = %#v", unreadable)
	}
	if node := graph.Nodes[broken]; node == nil || node.File != nil {
		t.Fatalf("broken node = %#v", graph.Nodes[broken])
	}

	empty := resolverFor(map[string]string{})
	missing, err := empty.Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !missing.Nodes[testConfig].Missing {
		t.Fatal("missing root file was not reported as missing")
	}
}

func TestResolveStopsAtMaxDepth(t *testing.T) {
	files := map[string]string{testConfig: "Include chain-0.conf\n"}
	for index := 0; index < 20; index++ {
		files[filepath.Join(testRoot, fmt.Sprintf("chain-%d.conf", index))] =
			fmt.Sprintf("Include chain-%d.conf\n", index+1)
	}
	graph, err := resolverFor(files).Resolve(testConfig)
	if err != nil {
		t.Fatal(err)
	}
	requireDiagnostic(t, graph, DiagnosticIncludeDepthExceeded)
	if len(graph.Nodes) != DefaultMaxDepth+1 {
		t.Fatalf("nodes = %d, want %d", len(graph.Nodes), DefaultMaxDepth+1)
	}
}

func TestResolveRejectsRelativeEntryPath(t *testing.T) {
	if _, err := resolverFor(nil).Resolve("config"); err == nil {
		t.Fatal("Resolve accepted a relative entry path")
	}
}
