package application

import (
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"sshc/internal/config"
)

// testRoot は、リゾルバが受け取るワークスペースのルートである。リゾルバは
// このファイルシステムの文法でパスを見るので、ここも組み立てて作る。
var testRoot = filepath.Join(testHome, ".ssh")

// fakeLoader は、射影テストが決してディスクに触れないように、
// メモリから設定ファイルを提供する。key は、本物の Loader が受け取るのと
// 同じ、このファイルシステムの絶対 path である。
type fakeLoader struct{ files map[string]string }

func (loader fakeLoader) ReadFile(name string) ([]byte, error) {
	contents, ok := loader.files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(contents), nil
}

func (loader fakeLoader) Glob(pattern string) ([]string, error) {
	var matches []string
	for name := range loader.files {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	return matches, nil
}

// newTestGraph は、メモリ内の設定 tree を解決する。key は testRoot
// からの相対で、スラッシュ区切りで書く——設定に書かれる綴りだからである。
func newTestGraph(t *testing.T, files map[string]string) *config.Graph {
	t.Helper()
	absolute := make(map[string]string, len(files))
	for name, contents := range files {
		absolute[filepath.Join(testRoot, filepath.FromSlash(name))] = contents
	}
	resolver := config.Resolver{
		Loader: fakeLoader{files: absolute},
		Home:   testHome,
		Root:   testRoot,
		Tokens: map[byte]string{'d': testHome},
	}
	graph, err := resolver.Resolve(filepath.Join(testRoot, "config"))
	if err != nil {
		t.Fatal(err)
	}
	return graph
}
