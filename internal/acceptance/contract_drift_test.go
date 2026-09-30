package acceptance_test

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"sshc/internal/api/contracttest"
	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// servedTypes は、その形のまま c.JSON へ渡される Go の型と、それが名乗っている
// openapi.yaml のスキーマの対である。
//
// これらの Go の形は 1 つしかない。以前は同じワイヤの形に対して
// internal/api にも生成された双子があり、しかも中身は揃っていなかった。
// 生成側は OpenAPI の省略可能を *T で表し、こちらは値と omitempty で表す。さらに
// こちらは DiffOp や EditAction のような名前付きの型を持ち、生成側はそれを string に
// する。寄せればドメインの側が弱くなるので、寄せずに生成しないことにした
// （api/oapi-codegen.yaml の exclude-schemas）。
//
// 双子が消えたぶん、突き合わせる相手は openapi.yaml そのものになった。契約の
// 写しではなく契約と比べるので、こちらの方が本来正しい。
//
// `make verify-generated` は「生成物が仕様と一致するか」しか見ない。手書きの型は、
// どれも internal/api/contracttest の同じ検査で、項目の名前・required・型・enum・入れ子まで
// openapi.yaml と突き合わせる。application の型はここに足し、httpserver が自分で持つ型は
// httpserver の wire_contract_test.go に足す。
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

// servedEnums は、application の型が OpenAPI の enum を名前付きの型で持つものの値である。
// 方式の名前は engine の backends の表から取る。httpserver の wire_contract_test.go の
// wireEnumValues も同じ表を使う。
var servedEnums = map[reflect.Type][]string{
	reflect.TypeFor[vpn.BackendName](): vpnBackendNames(),
}

// vpnBackendNames は、engine の backends の表にある方式の名前である。
func vpnBackendNames() []string {
	var names []string
	for _, backend := range vpn.Backends() {
		names = append(names, string(backend))
	}
	return names
}

// servedStringEnums は、application の型が OpenAPI の enum を名前の無い string で持つ場所と、
// その値をドメインが受け付けるかの確かめ方である。
var servedStringEnums = map[string]func(string) bool{
	"HostMetadata.os":                     acceptedAsHostMetadata(func(host *application.HostMetadata, value string) { host.OS = value }),
	"HostMetadata.detectedOS":             acceptedAsHostMetadata(func(host *application.HostMetadata, value string) { host.DetectedOS = value }),
	"HostMetadata.encoding":               acceptedAsHostMetadata(func(host *application.HostMetadata, value string) { host.Encoding = value }),
	"HostMetadata.osc52":                  acceptedAsHostMetadata(func(host *application.HostMetadata, value string) { host.OSC52 = value }),
	"VaultAutoLockSettings.mode":          oneOf(application.VaultAutoLockIdle, application.VaultAutoLockRestart),
	"VaultAutoLockSettings.unit":          oneOf(application.VaultAutoLockMinutes, application.VaultAutoLockHours),
	"PasswordEligibility.passwordBinding": oneOf(authenticationBindingStates...),
	"PasswordEligibility.totpBinding":     oneOf(authenticationBindingStates...),
}

// authenticationBindingStates は、Vault が返す、保存した認証情報と接続先の紐付けの状態である。
var authenticationBindingStates = []string{
	string(secret.AuthenticationBindingUnavailable), string(secret.AuthenticationBindingNone),
	string(secret.AuthenticationBindingCurrent), string(secret.AuthenticationBindingStale),
}

// acceptedAsHostMetadata は、1 つの接続の metadata の 1 項目に値を入れて、application が
// 保存できる metadata として受け付けるかを返す確かめ方を作る。
func acceptedAsHostMetadata(set func(host *application.HostMetadata, value string)) func(string) bool {
	return func(value string) bool {
		host := application.HostMetadata{Identity: application.HostIdentity{Path: "config", Alias: "example"}}
		set(&host, value)
		metadata := application.NewMetadata()
		metadata.Hosts = []application.HostMetadata{host}
		return application.ValidateMetadata(metadata) == nil
	}
}

// oneOf は、ドメインが定数で持つ値のどれかかを返す確かめ方を作る。
func oneOf(values ...string) func(string) bool {
	return func(value string) bool { return slices.Contains(values, value) }
}

func TestTheTypesWeSerialiseMatchTheContract(t *testing.T) {
	contract := contracttest.Contract{
		Document:    contracttest.ReadDocument(t, filepath.Join("..", "..")),
		Enums:       servedEnums,
		StringEnums: servedStringEnums,
		// 画面が送るキーの組は、application のショートカットの検査が確かめる。
		MapObjects: []string{"ShortcutPreset.bindings"},
	}
	for _, served := range servedTypes {
		t.Run(served.schema, func(t *testing.T) {
			contract.Verify(t, served.schema, served.value)
		})
	}
	// 確かめ方が何でも受け付けると、enum の検査は何も言わずに通る。
	for location, accepts := range servedStringEnums {
		if accepts("not-a-promised-value") {
			t.Errorf("%s: the domain check accepts a value that no enum promises", location)
		}
	}
}
