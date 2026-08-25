package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestEveryGeneratedModelIsReachableFromProduction prevents models-only
// generation from quietly accumulating operation aliases or component models
// which no Go handler uses. Tests are deliberately not roots: a model kept
// alive only by a contract fixture is still dead production code.
func TestEveryGeneratedModelIsReachableFromProduction(t *testing.T) {
	t.Helper()
	repository := filepath.Clean(filepath.Join("..", ".."))
	generatedPath := filepath.Join(repository, "internal", "api", "models.gen.go")
	fset := token.NewFileSet()
	generated, err := parser.ParseFile(fset, generatedPath, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	types := map[string]bool{}
	edges := map[string]map[string]bool{}
	for _, declaration := range generated.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range generic.Specs {
			switch value := specification.(type) {
			case *ast.TypeSpec:
				types[value.Name.Name] = true
				edges[value.Name.Name] = identifiersIn(value.Type)
			case *ast.ValueSpec:
				dependencies := identifiersIn(value)
				for _, name := range value.Names {
					edges[name.Name] = dependencies
				}
			}
		}
	}

	roots := map[string]bool{}
	err = filepath.WalkDir(repository, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "dist") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == generatedPath {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		aliases := map[string]bool{}
		for _, imported := range file.Imports {
			if strings.Trim(imported.Path.Value, "\"") != "sshc/internal/api" {
				continue
			}
			name := "api"
			if imported.Name != nil {
				name = imported.Name.Name
			}
			aliases[name] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			base, ok := selector.X.(*ast.Ident)
			if ok && aliases[base.Name] {
				roots[selector.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	reachable := map[string]bool{}
	queue := make([]string, 0, len(roots))
	for root := range roots {
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if reachable[name] {
			continue
		}
		reachable[name] = true
		for dependency := range edges[name] {
			if !reachable[dependency] {
				queue = append(queue, dependency)
			}
		}
	}

	var unreachable []string
	for name := range types {
		if !reachable[name] {
			unreachable = append(unreachable, name)
		}
	}
	sort.Strings(unreachable)
	if len(unreachable) > 0 {
		t.Fatalf("generated model types unreachable from production: %s", strings.Join(unreachable, ", "))
	}
}

func identifiersIn(node ast.Node) map[string]bool {
	result := map[string]bool{}
	ast.Inspect(node, func(child ast.Node) bool {
		if identifier, ok := child.(*ast.Ident); ok {
			result[identifier.Name] = true
		}
		return true
	})
	return result
}
