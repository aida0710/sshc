package acceptance_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"sshc/internal/api"
	"sshc/internal/application"
)

// pairedContractTypes は、同じ JSON の形を二度宣言している型の対である。
//
// **片方は openapi.yaml から生成され、もう片方は手で書かれている。** ドメインの値を
// そのまま c.JSON へ渡すエンドポイントがあるので、生成型が本番の Go から一度も
// 名指しされないまま、手書きの双子だけが実際の応答を決めている——そして
// `make verify-generated` が見るのは「生成物が仕様と一致するか」だけなので、
// **仕様を変えて手書きの方を直し忘れても、どこも赤くならなかった。**
//
// この検査は JSON のフィールド名の集合を突き合わせる。型を 1 つに寄せるまでの
// 間、少なくとも「気づかずにずれる」ことは無くなる。
//
// **application.TerminalSettings はここに居ない。** 名前は api の側と揃っているが、
// あれはワイヤに出ない——設定の書き込みだけに使う入力の型で、生成型から
// config_handlers が 1 項目ずつ詰め替えている。項目が増減すればコンパイルが
// 止まるので、名前の一致を見る必要が無い。実際、あの型には json タグが無く、
// もし直列化していれば PascalCase で出ていた。
var pairedContractTypes = []struct {
	name      string
	generated any
	served    any
}{
	{"ConflictReport", api.ConflictReport{}, application.ConflictReport{}},
	{"DiffLine", api.DiffLine{}, application.DiffLine{}},
	{"Effective", api.Effective{}, application.Effective{}},
	{"EffectiveChange", api.EffectiveChange{}, application.EffectiveChange{}},
	{"EffectiveDiff", api.EffectiveDiff{}, application.EffectiveDiff{}},
	{"EffectiveEntry", api.EffectiveEntry{}, application.EffectiveEntry{}},
	{"EmbeddedTerminal", api.EmbeddedTerminal{}, application.EmbeddedTerminal{}},
	{"FieldEdit", api.FieldEdit{}, application.FieldEdit{}},
	{"FileContents", api.FileContents{}, application.FileContents{}},
	{"FileDiff", api.FileDiff{}, application.FileDiff{}},
	{"FileNode", api.FileNode{}, application.FileNode{}},
	{"FileRef", api.FileRef{}, application.FileRef{}},
	{"FormField", api.FormField{}, application.FormField{}},
	{"GroupMetadata", api.GroupMetadata{}, application.GroupMetadata{}},
	{"GroupView", api.GroupView{}, application.GroupView{}},
	{"HistoryEntry", api.HistoryEntry{}, application.HistoryEntry{}},
	{"HostDetail", api.HostDetail{}, application.HostDetail{}},
	{"HostEntry", api.HostEntry{}, application.HostEntry{}},
	{"HostForm", api.HostForm{}, application.HostForm{}},
	{"HostIdentity", api.HostIdentity{}, application.HostIdentity{}},
	{"HostMetadata", api.HostMetadata{}, application.HostMetadata{}},
	{"IncludeReference", api.IncludeReference{}, application.IncludeReference{}},
	{"Metadata", api.Metadata{}, application.Metadata{}},
	{"Notice", api.Notice{}, application.Notice{}},
	{"Overview", api.Overview{}, application.Overview{}},
	{"PasswordEligibility", api.PasswordEligibility{}, application.PasswordEligibility{}},
	{"RelocatedKeyFile", api.RelocatedKeyFile{}, application.RelocatedKeyFile{}},
	{"RewrittenKeyReference", api.RewrittenKeyReference{}, application.RewrittenKeyReference{}},
	{"SavePreview", api.SavePreview{}, application.SavePreview{}},
	{"SaveResult", api.SaveResult{}, application.SaveResult{}},
	{"Setting", api.Setting{}, application.Setting{}},
	{"Source", api.Source{}, application.Source{}},
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

func TestGeneratedContractMatchesTheTypesWeSerialise(t *testing.T) {
	for _, pair := range pairedContractTypes {
		generated := jsonFieldNames(pair.generated)
		served := jsonFieldNames(pair.served)
		if slices.Equal(generated, served) {
			continue
		}
		t.Errorf("%s: openapi.yaml から生成した形と、実際に返している形が違う\n  生成: %v\n  応答: %v",
			pair.name, generated, served)
	}
}
