package httpserver

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// **この package が自分で綴る通信の型を、名前で並べない。**
//
// internal/acceptance の drift 検査は application の型を openapi.yaml と
// 突き合わせ、transport の検査は経路の一覧を突き合わせる。**その二つの網の
// あいだに、httpserver が自分で持っている型がある** ——application のものでは
// ないので前者に入らず、経路そのものではないので後者にも入らない。
//
// 実際そこで、problemPayload の blockers が長いあいだ契約に無いまま返り続けた。
// Problem は additionalProperties: false なので契約違反の本文であり、しかも
// 生成された型に入らないので**どのクライアントからも読めなかった。**
//
// **一覧を手で持つと、次に増えたものが同じ隙間に落ちる。** ソースから型を
// 拾い、契約に同じ名前のスキーマがあるものだけを突き合わせる。CLI と
// WebSocket の型（connectRequest、resizeMessage…）は /api/v1 の外に住んで
// いるので、対応するスキーマが無く、ここでは何も言わない。
func TestEveryWireTypeThisPackageSpellsMatchesTheContract(t *testing.T) {
	schemas := readSchemas(t)
	checked := 0

	for name, fields := range wireTypesIn(t, ".") {
		schema, found := schemas[schemaNameFor(name, schemas)]
		if !found {
			continue
		}
		promised := propertyNames(schema)
		if !slices.Equal(promised, fields) {
			t.Errorf("%s: openapi.yaml が約束している形と、実際に返している形が違う\n  契約: %v\n  応答: %v",
				name, promised, fields)
		}
		// **additionalProperties が false であることまで見る。** 緩めば契約は
		// 「知らない項目を返してよい」と言い出し、上の比較の意味が消える。
		if allowed, _ := schema["additionalProperties"].(bool); allowed {
			t.Errorf("%s allows unknown members, so comparing it means nothing", name)
		}
		checked++
	}

	// **ゼロ件で緑にならない。** 型の拾い方を壊したら、この検査は何も言わずに
	// 通る——それは、隙間をもう一度作ることである。
	if checked < 2 {
		t.Errorf("only %d wire types were compared; the way they are collected is broken", checked)
	}
}

func readSchemas(t *testing.T) map[string]map[string]any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	components, _ := spec["components"].(map[string]any)
	raw, _ := components["schemas"].(map[string]any)
	schemas := make(map[string]map[string]any, len(raw))
	for name, value := range raw {
		if schema, ok := value.(map[string]any); ok {
			schemas[name] = schema
		}
	}
	return schemas
}

func propertyNames(schema map[string]any) []string {
	properties, _ := schema["properties"].(map[string]any)
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// schemaNameFor は、Go の型名に対応するスキーマの名前を探す。
//
// **payload・request・response の接尾辞は Go 側の都合である。** 契約の側は
// Problem であって problemPayload ではない。
func schemaNameFor(gotype string, schemas map[string]map[string]any) string {
	lowered := strings.ToLower(gotype)
	trimmed := lowered
	for _, suffix := range []string{"payload", "body"} {
		trimmed = strings.TrimSuffix(trimmed, suffix)
	}
	for name := range schemas {
		if lower := strings.ToLower(name); lower == lowered || lower == trimmed {
			return name
		}
	}
	return ""
}

// wireTypesIn は、この package が綴る struct の、JSON へ出す名前を集める。
func wireTypesIn(t *testing.T, directory string) map[string][]string {
	t.Helper()
	set := token.NewFileSet()
	packages, err := parser.ParseDir(set, directory, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]string{}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				spec, ok := node.(*ast.TypeSpec)
				if !ok {
					return true
				}
				structure, ok := spec.Type.(*ast.StructType)
				if !ok {
					return true
				}
				var names []string
				for _, field := range structure.Fields.List {
					if field.Tag == nil {
						continue
					}
					tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
					name, _, _ := strings.Cut(tag.Get("json"), ",")
					if name != "" && name != "-" {
						names = append(names, name)
					}
				}
				if len(names) != 0 {
					slices.Sort(names)
					found[spec.Name.Name] = names
				}
				return true
			})
		}
	}
	return found
}
