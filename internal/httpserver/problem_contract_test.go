package httpserver

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// **断りの本文も、契約の一部である。**
//
// internal/acceptance の drift 検査は application の型を openapi.yaml と
// 突き合わせるが、**断りを綴るのは httpserver の problemPayload であり、
// あちらの一覧には入らない。** その隙間で、blockers という項目が長いあいだ
// 契約に無いまま返り続けていた——Problem は additionalProperties: false なので
// 契約違反の本文であり、しかも生成された型に入らないので**どのクライアントからも
// 読めなかった。** 「なぜ断られたか」を出しているつもりで、誰にも届いていない。
//
// ここが数えるのは、その隙間である。
func TestTheRefusalWeSerialiseMatchesTheContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	problem, found := schemas["Problem"].(map[string]any)
	if !found {
		t.Fatal("openapi.yaml に Problem が無い")
	}

	// **additionalProperties: false であることまで確かめる。** これが緩めば、
	// 契約は「知らない項目を返してよい」と言い出し、この検査の意味が消える。
	if allowed, _ := problem["additionalProperties"].(bool); allowed {
		t.Error("Problem allows unknown members, so this check no longer means anything")
	}

	properties, _ := problem["properties"].(map[string]any)
	promised := make([]string, 0, len(properties))
	for name := range properties {
		promised = append(promised, name)
	}
	slices.Sort(promised)

	structure := reflect.TypeOf(problemPayload{})
	actual := make([]string, 0, structure.NumField())
	for index := range structure.NumField() {
		name, _, _ := strings.Cut(structure.Field(index).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		actual = append(actual, name)
	}
	slices.Sort(actual)

	if !slices.Equal(promised, actual) {
		t.Errorf("Problem: openapi.yaml が約束している形と、実際に断りで返している形が違う\n  契約: %v\n  応答: %v",
			promised, actual)
	}
}
