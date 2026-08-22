package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"sshc/internal/application"
)

// servedTypes は、その形のまま c.JSON へ渡される Go の型と、それが名乗っている
// openapi.yaml のスキーマの対である。
//
// **これらの Go の形は 1 つしかない。** 以前は同じワイヤの形に対して
// internal/api にも生成された双子があり、**しかも中身は揃っていなかった** ——
// 生成側は OpenAPI の省略可能を *T で表し、こちらは値と omitempty で表す。さらに
// こちらは DiffOp や EditAction のような名前付きの型を持ち、生成側はそれを string に
// する。寄せればドメインの側が弱くなるので、寄せずに**生成しない**ことにした
// （api/oapi-codegen.yaml の exclude-schemas）。
//
// 双子が消えたぶん、突き合わせる相手は openapi.yaml そのものになった。**契約の
// 写しではなく契約と比べる**ので、こちらの方が本来正しい。
//
// `make verify-generated` は「生成物が仕様と一致するか」しか見ない。**実際に返る
// JSON が仕様と一致するかを見るのは、ここだけである。**
var servedTypes = []struct {
	schema string
	value  any
}{
	{"ConflictReport", application.ConflictReport{}},
	{"CreateConnectionResponse", application.CreateConnectionResult{}},
	{"DiffLine", application.DiffLine{}},
	{"Effective", application.Effective{}},
	{"EffectiveChange", application.EffectiveChange{}},
	{"EffectiveDiff", application.EffectiveDiff{}},
	{"EffectiveEntry", application.EffectiveEntry{}},
	{"EmbeddedTerminal", application.EmbeddedTerminal{}},
	{"FieldEdit", application.FieldEdit{}},
	{"FileContents", application.FileContents{}},
	{"FileDiff", application.FileDiff{}},
	{"FileNode", application.FileNode{}},
	{"FileRef", application.FileRef{}},
	{"FormField", application.FormField{}},
	{"GroupMetadata", application.GroupMetadata{}},
	{"GroupView", application.GroupView{}},
	{"HistoryEntry", application.HistoryEntry{}},
	{"HostDetail", application.HostDetail{}},
	{"HostEntry", application.HostEntry{}},
	{"HostForm", application.HostForm{}},
	{"HostIdentity", application.HostIdentity{}},
	{"HostMetadata", application.HostMetadata{}},
	{"IncludeReference", application.IncludeReference{}},
	{"Metadata", application.Metadata{}},
	{"Notice", application.Notice{}},
	{"Overview", application.Overview{}},
	{"PasswordEligibility", application.PasswordEligibility{}},
	{"RelocatedKeyFile", application.RelocatedKeyFile{}},
	{"RewrittenKeyReference", application.RewrittenKeyReference{}},
	{"SavePreview", application.SavePreview{}},
	{"SaveResult", application.SaveResult{}},
	{"Setting", application.Setting{}},
	{"Source", application.Source{}},
	{"TerminalAppearance", application.TerminalAppearance{}},
}

// jsonFieldNames は、その型が JSON へ出す名前を集める。
//
// `-` は出ないので数えない。omitempty は名前を変えないので落とす——見たいのは
// 「どの名前が出るか」であって、空のときに省かれるかどうかではない。
func jsonFieldNames(value any) []string {
	structure := reflect.TypeOf(value)
	names := make([]string, 0, structure.NumField())
	for index := range structure.NumField() {
		field := structure.Field(index)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		switch name {
		case "-":
			continue
		case "":
			name = field.Name
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// schemaProperties は、openapi.yaml のスキーマひとつが約束している名前を返す。
func schemaProperties(t *testing.T, spec map[string]any, name string) []string {
	t.Helper()
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	schema, found := schemas[name].(map[string]any)
	if !found {
		t.Fatalf("openapi.yaml に %s というスキーマが無い", name)
	}
	properties, found := schema["properties"].(map[string]any)
	if !found {
		t.Fatalf("%s が properties を持っていない", name)
	}
	names := make([]string, 0, len(properties))
	for property := range properties {
		names = append(names, property)
	}
	slices.Sort(names)
	return names
}

func TestTheTypesWeSerialiseMatchTheContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}

	for _, served := range servedTypes {
		promised := schemaProperties(t, spec, served.schema)
		actual := jsonFieldNames(served.value)
		if slices.Equal(promised, actual) {
			continue
		}
		t.Errorf("%s: openapi.yaml が約束している形と、実際に返している形が違う\n  契約: %v\n  応答: %v",
			served.schema, promised, actual)
	}
}
