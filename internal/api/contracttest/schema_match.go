// Package contracttest は、手書きの Go の型が api/openapi.yaml のスキーマと同じ形かを、
// 項目の名前・required・型・enum・入れ子まで再帰的に確かめる。テストからだけ使う。
//
// oapi-codegen が Go の型を生成しないスキーマ（api/oapi-codegen.yaml の exclude-schemas）は、
// application・httpserver などの手書きの型が正本になる。どの package の型も、この package の
// 同じ検査で openapi.yaml と突き合わせる。
package contracttest

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const schemaReferencePrefix = "#/components/schemas/"

// Document は、api/openapi.yaml のうち、型の突き合わせに使う components.schemas である。
type Document struct {
	Components struct {
		Schemas map[string]map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

// ReadDocument は、repositoryRoot（テストの package からリポジトリの根への相対パス）にある
// api/openapi.yaml を読む。
func ReadDocument(t testing.TB, repositoryRoot string) Document {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repositoryRoot, "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document Document
	if err := yaml.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

// Contract は、手書きの Go の型を openapi.yaml と突き合わせるときの決まりである。
type Contract struct {
	Document Document
	// Enums は、OpenAPI の enum を Go で表す名前付きの型ごとに、その型が取りうる値を並べる。
	// enum を持つ項目の Go の型がここに無ければ、検査は失敗する。
	Enums map[reflect.Type][]string
	// StringEnums は、OpenAPI の enum を名前の無い string で持つ場所ごとに（"スキーマ名.項目名"）、
	// その値をドメインが受け付けるかを返す。OpenAPI が約束する値をドメインが断らないことは
	// 確かめられるが、OpenAPI に無い値をドメインが返さないことまでは確かめられない。
	StringEnums map[string]func(value string) bool
	// MapObjects は、決まった項目を持つ object を Go では map[string]T で持つ場所を、
	// "スキーマ名.項目名" で並べる。どのキーを受け付けるかは、Go ではドメインが確かめる。
	MapObjects []string
}

// Verify は、schema という名前のスキーマと value の型を突き合わせ、食い違いを t に報告する。
func (c Contract) Verify(t testing.TB, schema string, value any) {
	t.Helper()
	walk := schemaWalk{t: t, contract: c, visited: map[visit]bool{}}
	walk.verify(location{schema: schema}, map[string]any{"$ref": schemaReferencePrefix + schema}, reflect.TypeOf(value))
}

// location は、食い違いを報告する場所である。schema は最も近い名前付きのスキーマ、
// path はそこからの項目のたどり方（"bindings"、"hosts[]"、"labels{}" など）である。
type location struct {
	schema string
	path   string
}

func (l location) String() string {
	if l.path == "" || strings.HasPrefix(l.path, "[") || strings.HasPrefix(l.path, "{") {
		return l.schema + l.path
	}
	return l.schema + "." + l.path
}

func (l location) property(name string) location {
	if l.path == "" {
		return location{schema: l.schema, path: name}
	}
	return location{schema: l.schema, path: l.path + "." + name}
}

// items は、配列の要素の場所である。
func (l location) items() location {
	return location{schema: l.schema, path: l.path + "[]"}
}

// mapValues は、キーを決めない object（Go の map）の値の場所である。
func (l location) mapValues() location {
	return location{schema: l.schema, path: l.path + "{}"}
}

type visit struct {
	schema string
	typeID reflect.Type
}

type jsonField struct {
	typeID   reflect.Type
	required bool
}

// schemaWalk は、1 つの型を突き合わせるあいだの状態である。visited は、再帰する
// スキーマ（木構造の node など）で同じ組を二度たどらないために持つ。
type schemaWalk struct {
	t        testing.TB
	contract Contract
	visited  map[visit]bool
}

func (w schemaWalk) verify(at location, schema map[string]any, typeID reflect.Type) {
	w.t.Helper()
	for typeID.Kind() == reflect.Pointer {
		typeID = typeID.Elem()
	}
	if reference, ok := schema["$ref"].(string); ok {
		identity := visit{schema: reference, typeID: typeID}
		if w.visited[identity] {
			return
		}
		w.visited[identity] = true
		at = location{schema: strings.TrimPrefix(reference, schemaReferencePrefix)}
	}

	resolved := w.resolve(schema)
	schemaType, _ := resolved["type"].(string)
	if schemaType == "" && resolved["properties"] != nil {
		schemaType = "object"
	}
	switch schemaType {
	case "object":
		w.verifyObject(at, resolved, typeID)
	case "array":
		if typeID.Kind() != reflect.Slice && typeID.Kind() != reflect.Array {
			w.t.Fatalf("%s: OpenAPI array is represented by %v", at, typeID)
		}
		items, ok := resolved["items"].(map[string]any)
		if !ok {
			w.t.Fatalf("%s: array schema has no items", at)
		}
		w.verify(at.items(), items, typeID.Elem())
	case "string":
		if typeID != reflect.TypeFor[time.Time]() && typeID.Kind() != reflect.String {
			w.t.Fatalf("%s: OpenAPI string is represented by %v", at, typeID)
		}
		w.verifyEnum(at, resolved, typeID)
	case "integer":
		if typeID.Kind() < reflect.Int || typeID.Kind() > reflect.Uint64 {
			w.t.Fatalf("%s: OpenAPI integer is represented by %v", at, typeID)
		}
		if format, _ := resolved["format"].(string); format == "int64" && typeID.Kind() != reflect.Int64 {
			w.t.Fatalf("%s: OpenAPI int64 is represented by %v", at, typeID)
		}
	case "number":
		if typeID.Kind() != reflect.Float32 && typeID.Kind() != reflect.Float64 {
			w.t.Fatalf("%s: OpenAPI number is represented by %v", at, typeID)
		}
		if format, _ := resolved["format"].(string); format == "double" && typeID.Kind() != reflect.Float64 {
			w.t.Fatalf("%s: OpenAPI double is represented by %v", at, typeID)
		}
	case "boolean":
		if typeID.Kind() != reflect.Bool {
			w.t.Fatalf("%s: OpenAPI boolean is represented by %v", at, typeID)
		}
	default:
		w.t.Fatalf("%s: unsupported or missing OpenAPI type %q in %#v", at, schemaType, resolved)
	}
}

func (w schemaWalk) verifyObject(at location, schema map[string]any, typeID reflect.Type) {
	w.t.Helper()
	if additional, exists := schema["additionalProperties"]; exists && additional != false {
		if typeID.Kind() != reflect.Map || typeID.Key().Kind() != reflect.String {
			w.t.Fatalf("%s: OpenAPI string map is represented by %v", at, typeID)
		}
		valueSchema, ok := additional.(map[string]any)
		if !ok {
			w.t.Fatalf("%s: unsupported additionalProperties %#v", at, additional)
		}
		w.verify(at.mapValues(), valueSchema, typeID.Elem())
		return
	}
	properties, _ := schema["properties"].(map[string]any)
	if typeID.Kind() == reflect.Map && slices.Contains(w.contract.MapObjects, at.String()) {
		w.verifyMapObject(at, properties, typeID)
		return
	}
	if typeID.Kind() != reflect.Struct {
		w.t.Fatalf("%s: OpenAPI object is represented by %v", at, typeID)
	}
	required := stringSet(schema["required"])
	fields := w.collectJSONFields(at, typeID)
	for name, property := range properties {
		field, ok := fields[name]
		if !ok {
			w.t.Errorf("%s: OpenAPI property has no Go JSON field", at.property(name))
			continue
		}
		if field.required != required[name] {
			w.t.Errorf("%s: required=%t but Go omitempty implies required=%t", at.property(name), required[name], field.required)
		}
		propertySchema, ok := property.(map[string]any)
		if !ok {
			w.t.Errorf("%s: invalid schema %#v", at.property(name), property)
			continue
		}
		w.verify(at.property(name), propertySchema, field.typeID)
	}
	for name := range fields {
		if _, ok := properties[name]; !ok {
			w.t.Errorf("%s: Go JSON field is absent from OpenAPI", at.property(name))
		}
	}
}

// verifyMapObject は、Contract.MapObjects に挙げた object の、どの項目のスキーマも
// map の値の型で表せることを確かめる。
func (w schemaWalk) verifyMapObject(at location, properties map[string]any, typeID reflect.Type) {
	w.t.Helper()
	if typeID.Key().Kind() != reflect.String {
		w.t.Fatalf("%s: OpenAPI object is represented by %v", at, typeID)
	}
	for name, property := range properties {
		propertySchema, ok := property.(map[string]any)
		if !ok {
			w.t.Errorf("%s: invalid schema %#v", at.property(name), property)
			continue
		}
		w.verify(at.property(name), propertySchema, typeID.Elem())
	}
}

func (w schemaWalk) collectJSONFields(at location, typeID reflect.Type) map[string]jsonField {
	w.t.Helper()
	fields := make(map[string]jsonField)
	for index := range typeID.NumField() {
		field := typeID.Field(index)
		tag := field.Tag.Get("json")
		name, options, _ := strings.Cut(tag, ",")
		if name == "-" || (!field.IsExported() && !field.Anonymous) {
			continue
		}
		if field.Anonymous && name == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() != reflect.Struct {
				w.t.Fatalf("%s: unsupported anonymous JSON field %v", at, field.Type)
			}
			for embeddedName, embeddedField := range w.collectJSONFields(at, embedded) {
				if _, duplicate := fields[embeddedName]; duplicate {
					w.t.Fatalf("%s: duplicate JSON field", at.property(embeddedName))
				}
				fields[embeddedName] = embeddedField
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		if _, duplicate := fields[name]; duplicate {
			w.t.Fatalf("%s: duplicate JSON field", at.property(name))
		}
		fields[name] = jsonField{typeID: field.Type, required: !slices.Contains(strings.Split(options, ","), "omitempty")}
	}
	return fields
}

func (w schemaWalk) verifyEnum(at location, schema map[string]any, typeID reflect.Type) {
	w.t.Helper()
	items := anySlice(schema["enum"])
	if len(items) == 0 {
		return
	}
	promised := make([]string, 0, len(items))
	for _, item := range items {
		promised = append(promised, fmt.Sprint(item))
	}
	slices.Sort(promised)
	if typeID == reflect.TypeFor[string]() {
		w.verifyStringEnum(at, promised)
		return
	}
	actual, ok := w.contract.Enums[typeID]
	if !ok {
		w.t.Fatalf("%s: OpenAPI enum is represented by unregistered Go type %v", at, typeID)
	}
	actual = slices.Sorted(slices.Values(actual))
	if !slices.Equal(promised, actual) {
		w.t.Errorf("%s: enum differs: OpenAPI=%v Go=%v", at, promised, actual)
	}
}

func (w schemaWalk) verifyStringEnum(at location, promised []string) {
	w.t.Helper()
	accepts, ok := w.contract.StringEnums[at.String()]
	if !ok {
		w.t.Fatalf("%s: OpenAPI enum is represented by a plain Go string; register the domain's check in StringEnums", at)
	}
	for _, value := range promised {
		if !accepts(value) {
			w.t.Errorf("%s: OpenAPI promises %q, but the domain refuses it", at, value)
		}
	}
}

// resolve は、$ref と allOf をたどって、1 つのスキーマにまとめる。
func (w schemaWalk) resolve(schema map[string]any) map[string]any {
	w.t.Helper()
	resolved := make(map[string]any)
	if reference, ok := schema["$ref"].(string); ok {
		name := strings.TrimPrefix(reference, schemaReferencePrefix)
		if name == reference {
			w.t.Fatalf("unsupported schema reference %q", reference)
		}
		definition, exists := w.contract.Document.Components.Schemas[name]
		if !exists {
			w.t.Fatalf("unknown schema reference %q", reference)
		}
		w.merge(resolved, definition)
	}
	w.merge(resolved, schema)
	return resolved
}

func (w schemaWalk) merge(destination, source map[string]any) {
	w.t.Helper()
	if allOf, ok := source["allOf"].([]any); ok {
		for _, part := range allOf {
			partSchema, ok := part.(map[string]any)
			if !ok {
				w.t.Fatalf("invalid allOf part %#v", part)
			}
			mergeResolvedSchema(destination, w.resolve(partSchema))
		}
	}
	own := make(map[string]any)
	for key, value := range source {
		if key != "$ref" && key != "allOf" {
			own[key] = value
		}
	}
	mergeResolvedSchema(destination, own)
}

func mergeResolvedSchema(destination, source map[string]any) {
	for key, value := range source {
		switch key {
		case "properties":
			properties, _ := destination[key].(map[string]any)
			if properties == nil {
				properties = make(map[string]any)
				destination[key] = properties
			}
			for name, property := range value.(map[string]any) {
				properties[name] = property
			}
		case "required":
			destination[key] = slices.Concat(anySlice(destination[key]), anySlice(value))
		default:
			destination[key] = value
		}
	}
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func stringSet(value any) map[string]bool {
	result := make(map[string]bool)
	for _, item := range anySlice(value) {
		if text, ok := item.(string); ok {
			result[text] = true
		}
	}
	return result
}
