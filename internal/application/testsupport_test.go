package application

import (
	"io/fs"
	"path/filepath"
	"sort"
	"testing"

	"sshc/internal/config"
)

var testRoot = filepath.Join(testHome, ".ssh")

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
