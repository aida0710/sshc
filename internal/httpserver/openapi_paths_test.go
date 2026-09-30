package httpserver

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// readOpenAPIPaths は、api/openapi.yaml の paths を返す。パスごとに、"get" などの操作と
// 共通の "parameters" を node のまま引ける。
func readOpenAPIPaths(t *testing.T) map[string]map[string]yaml.Node {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	return document.Paths
}
