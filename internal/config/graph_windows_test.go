//go:build windows

package config

import (
	"errors"
	"testing"
)

// これが通らない限り、Windows では設定がひとつも読めない。エントリのパスは
// ワークスペースから来るネイティブなパスであり、`C:\` も `\\server\share\` も
// 絶対パスである。
func TestWindowsResolveAcceptsDriveAndUNCEntries(t *testing.T) {
	for name, test := range map[string]struct{ home, root, entry string }{
		"drive": {`C:\Users\Tester`, `C:\Users\Tester\.ssh`, `C:\Users\Tester\.ssh\config`},
		"unc":   {`\\server\share\Tester`, `\\server\share\Tester\.ssh`, `\\server\share\Tester\.ssh\config`},
	} {
		t.Run(name, func(t *testing.T) {
			resolver := Resolver{
				Loader: fakeLoader{files: map[string]string{test.entry: "Host example\n"}},
				Home:   test.home,
				Root:   test.root,
			}
			graph, err := resolver.Resolve(test.entry)
			if err != nil {
				t.Fatalf("Resolve(%q) = %v", test.entry, err)
			}
			node := graph.Nodes[test.entry]
			if node == nil || node.File == nil {
				t.Fatalf("entry node = %#v", node)
			}
			if !node.Editable {
				t.Fatal("the entry file under the workspace root was not editable")
			}
		})
	}
}

func TestWindowsResolveRefusesUnsupportedEntrySpellings(t *testing.T) {
	resolver := Resolver{
		Loader: fakeLoader{},
		Home:   `C:\Users\Tester`,
		Root:   `C:\Users\Tester\.ssh`,
	}
	for _, entry := range []string{
		`C:config`,
		`\config`,
		`config`,
		`\\?\C:\Users\Tester\.ssh\config`,
		`\\.\C:\Users\Tester\.ssh\config`,
		`\??\C:\Users\Tester\.ssh\config`,
		"",
	} {
		if _, err := resolver.Resolve(entry); !errors.Is(err, ErrPathNotAbsolute) {
			t.Errorf("Resolve(%q) = %v, want ErrPathNotAbsolute", entry, err)
		}
	}
}

// 大小文字だけが違う表記は同じファイルである。別々に読み込めば同じ内容が二重に
// 現れ、循環はいつまでも見つからない。
func TestWindowsResolveTreatsCaseAliasesAsOneFile(t *testing.T) {
	const root = `C:\Users\Tester\.ssh`
	const entry = root + `\config`
	const extra = root + `\extra.conf`
	resolver := Resolver{
		Loader: fakeLoader{
			files: map[string]string{
				entry: "Include a.conf\nInclude b.conf\n",
				extra: "Host extra\n",
			},
			globs: map[string][]string{
				root + `\a.conf`: {extra},
				root + `\b.conf`: {`c:\users\tester\.ssh\EXTRA.CONF`},
			},
		},
		Home: `C:\Users\Tester`,
		Root: root,
	}

	graph, err := resolver.Resolve(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Order) != 2 {
		t.Fatalf("graph order = %#v, want the entry and one included file", graph.Order)
	}
	node := graph.Nodes[extra]
	if node == nil {
		t.Fatalf("nodes = %#v, want the first spelling to own the node", graph.Nodes)
	}
	if node.Loads != 2 {
		t.Fatalf("loads = %d, want 2", node.Loads)
	}
	if !hasDiagnostic(graph, DiagnosticIncludeDuplicate) {
		t.Fatalf("diagnostics = %#v, want a duplicate report", graph.Diagnostics)
	}
}

func TestWindowsResolveDetectsACaseAliasCycleOnce(t *testing.T) {
	const root = `C:\Users\Tester\.ssh`
	const entry = root + `\config`
	resolver := Resolver{
		Loader: fakeLoader{
			files: map[string]string{entry: "Include self.conf\n"},
			globs: map[string][]string{root + `\self.conf`: {`c:\users\tester\.ssh\CONFIG`}},
		},
		Home: `C:\Users\Tester`,
		Root: root,
	}

	graph, err := resolver.Resolve(entry)
	if err != nil {
		t.Fatal(err)
	}
	cycles := 0
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == DiagnosticIncludeCycle {
			cycles++
		}
	}
	if cycles != 1 {
		t.Fatalf("cycle diagnostics = %d, want exactly 1", cycles)
	}
	if len(graph.Order) != 1 {
		t.Fatalf("graph order = %#v, want only the entry", graph.Order)
	}
}

// 別のボリュームや別の共有は、表記が似ていてもワークスペースの外である。
// 読むことはできるが、書き換えてよい場所ではない。
func TestWindowsResolveKeepsOtherVolumesAndSiblingsOutsideTheRoot(t *testing.T) {
	const root = `C:\Users\Tester\.ssh`
	const entry = root + `\config`
	outside := []string{`D:\shared\ssh_config`, `C:\Users\Tester\.ssh-other\ssh_config`}
	files := map[string]string{entry: "Include a.conf\nInclude b.conf\n"}
	for _, path := range outside {
		files[path] = "Host elsewhere\n"
	}
	resolver := Resolver{
		Loader: fakeLoader{
			files: files,
			globs: map[string][]string{
				root + `\a.conf`: {outside[0]},
				root + `\b.conf`: {outside[1]},
			},
		},
		Home: `C:\Users\Tester`,
		Root: root,
	}

	graph, err := resolver.Resolve(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range outside {
		node := graph.Nodes[path]
		if node == nil {
			t.Fatalf("nodes = %#v, want %q to be read for display", graph.Nodes, path)
		}
		if node.Editable {
			t.Fatalf("%q was reported editable", path)
		}
	}
	if !hasDiagnostic(graph, DiagnosticIncludeOutsideRoot) {
		t.Fatalf("diagnostics = %#v, want an outside-root report", graph.Diagnostics)
	}
}

func hasDiagnostic(graph *Graph, code string) bool {
	for _, diagnostic := range graph.Diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

// 大小文字だけが違う表記で include されたルート下のファイルは、ルート自身の
// 表記で引ける節点にならなければならない。
//
// そうでないと、一覧に出て編集可能と言われたホストが、開いた瞬間に
// 見つからないと言われる。上の層はルートから組み立てた表記で引いている。
func TestWindowsResolveKeysInRootNodesByTheRootSpelling(t *testing.T) {
	const root = `C:\Users\Tester\.ssh`
	const entry = root + `\config`
	const shouted = `C:\USERS\TESTER\.ssh\conf.d\work.conf`
	resolver := Resolver{
		Loader: fakeLoader{
			// 本物のファイルシステムはどちらの表記にも返す。偽物には両方
			// 登録しておく。確かめたいのは節点の鍵であって、偽物の索き方ではない。
			files: map[string]string{
				entry:                      "Include conf.d/*.conf\n",
				shouted:                    "Host work\n",
				root + `\conf.d\work.conf`: "Host work\n",
			},
			globs: map[string][]string{root + `\conf.d\*.conf`: {shouted}},
		},
		Home: `C:\Users\Tester`,
		Root: root,
	}

	graph, err := resolver.Resolve(entry)
	if err != nil {
		t.Fatal(err)
	}
	expected := root + `\conf.d\work.conf`
	node := graph.Nodes[expected]
	if node == nil {
		t.Fatalf("nodes = %#v, want a node at %q", graph.Nodes, expected)
	}
	if !node.Editable {
		t.Fatal("a file under the workspace root was not editable")
	}
	if node.File == nil {
		t.Fatal("the canonical node was never read")
	}
}

// 辺に載る一致も、節点の鍵と同じ表記でなければならない。
//
// 片方だけ揃えると、同じファイルが二つの名前で現れる。辺をたどる側は
// その節点を見つけられず、include されたファイルは丸ごと無かったことになる。
func TestWindowsResolveCanonicalisesTheMatchesItReports(t *testing.T) {
	const root = `C:\Users\Tester\.ssh`
	const entry = root + `\config`
	const shouted = `C:\USERS\TESTER\.ssh\conf.d\work.conf`
	const canonical = root + `\conf.d\work.conf`
	resolver := Resolver{
		Loader: fakeLoader{
			files: map[string]string{
				entry:     "Include conf.d/*.conf\n",
				shouted:   "Host work\n",
				canonical: "Host work\n",
			},
			globs: map[string][]string{root + `\conf.d\*.conf`: {shouted}},
		},
		Home: `C:\Users\Tester`,
		Root: root,
	}

	graph, err := resolver.Resolve(entry)
	if err != nil {
		t.Fatal(err)
	}
	entryNode := graph.Nodes[entry]
	if entryNode == nil || len(entryNode.Includes) != 1 {
		t.Fatalf("entry node = %#v", entryNode)
	}
	matches := entryNode.Includes[0].Matches
	if len(matches) != 1 || matches[0] != canonical {
		t.Fatalf("edge matches = %#v, want %q", matches, canonical)
	}
	if graph.Nodes[matches[0]] == nil {
		t.Fatalf("the edge names %q, which is not a node", matches[0])
	}
}
