package vpn

import (
	"reflect"
	"strings"
	"testing"
)

// 方式ごとの並びのうち、backends の表の外に手で書いたものが、どの方式も漏らさずに
// 扱うことを確かめる。並びの一覧は backend.go の backend のコメントにある。

// sectionFields は、構造体の型のうち、方式ごとの節（構造体へのポインタ）の項目の番号を返す。
func sectionFields(structType reflect.Type) []int {
	var fields []int
	for index := range structType.NumField() {
		field := structType.Field(index)
		if field.Type.Kind() == reflect.Pointer && field.Type.Elem().Kind() == reflect.Struct {
			fields = append(fields, index)
		}
	}
	return fields
}

// profileWithSection は、backend を名乗り、index の節だけを空の値で持つプロファイルである。
func profileWithSection(backend BackendName, index int) Profile {
	profile := Profile{Backend: backend}
	section := reflect.ValueOf(&profile).Elem().Field(index)
	section.Set(reflect.New(section.Type().Elem()))
	return profile
}

// Profile は方式ごとに節をひとつ持ち、foreignSection はどの方式でも、ほかの方式の節を
// 見つける。足し忘れると、方式を切り替えたあとの古い節が経路の設定に紛れる。
func TestEveryBackendOwnsExactlyOneProfileSection(t *testing.T) {
	fields := sectionFields(reflect.TypeFor[Profile]())
	if len(fields) != len(backends) {
		t.Fatalf("Profile の節 = %d、backends の表 = %d", len(fields), len(backends))
	}
	owners := map[int]BackendName{}
	for _, backend := range Backends() {
		var owned []int
		for _, index := range fields {
			if profileWithSection(backend, index).foreignSection() == "" {
				owned = append(owned, index)
			}
		}
		if len(owned) != 1 {
			t.Errorf("%s が自分の節と見なす Profile の節 = %d 個", backend, len(owned))
			continue
		}
		if other, taken := owners[owned[0]]; taken {
			t.Errorf("%s と %s が同じ節を自分のものと見なす", backend, other)
		}
		owners[owned[0]] = backend
	}
}

// SecretsDocument のどの項目も、方式ごとの型を経て同じ値に戻り、Secrets の節はどの方式も
// 埋まる。足し忘れると、その項目は Vault へ書かれずに消える。
func TestEverySecretOfTheDocumentSurvivesTheBackendTypes(t *testing.T) {
	var document SecretsDocument
	fields := reflect.ValueOf(&document).Elem()
	for index := range fields.NumField() {
		fields.Field(index).SetString("value of " + fields.Type().Field(index).Name)
	}

	secrets := document.Secrets()

	if back := secrets.Document(); back != document {
		t.Errorf("Secrets().Document() = %+v, want %+v", back, document)
	}
	sections := sectionFields(reflect.TypeFor[Secrets]())
	if len(sections) != len(backends) {
		t.Fatalf("Secrets の節 = %d、backends の表 = %d", len(sections), len(backends))
	}
	for _, index := range sections {
		if reflect.ValueOf(secrets).Field(index).IsNil() {
			t.Errorf("Secrets の %s が作られない", reflect.TypeFor[Secrets]().Field(index).Name)
		}
	}
}

// イメージは、どの方式の手順も焼き込む。漏れると、その方式だけがコンテナの中で
// 手順を読めずに止まる。
func TestTheImageCopiesEveryBackendScript(t *testing.T) {
	dockerfile, err := container.ReadFile("container/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range Backends() {
		if !strings.Contains(string(dockerfile), "backend-"+string(backend)+".sh") {
			t.Errorf("Dockerfile が backend-%s.sh を COPY しない", backend)
		}
	}
}
