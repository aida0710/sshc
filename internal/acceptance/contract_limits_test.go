package acceptance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"sshc/internal/remotesync"
	"sshc/internal/sftp"
	"sshc/internal/validate"
	"sshc/internal/workspace"
)

// openAPISpec は、api/openapi.yaml を型の無い木として読む。
func openAPISpec(t *testing.T) map[string]any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := yaml.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

// openAPIProperty は、スキーマ名から "." 区切りのプロパティの道をたどった先の
// スキーマを返す。途中の $ref は components/schemas の中へ解決する。
func openAPIProperty(t *testing.T, spec map[string]any, schema, path string) map[string]any {
	t.Helper()
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	resolve := func(node map[string]any) map[string]any {
		if reference, ok := node["$ref"].(string); ok {
			resolved, found := schemas[strings.TrimPrefix(reference, "#/components/schemas/")].(map[string]any)
			if !found {
				t.Fatalf("openapi.yaml の %s が指すスキーマが無い", reference)
			}
			return resolved
		}
		return node
	}
	node, found := schemas[schema].(map[string]any)
	if !found {
		t.Fatalf("openapi.yaml に %s というスキーマが無い", schema)
	}
	for _, name := range strings.Split(path, ".") {
		properties, _ := resolve(node)["properties"].(map[string]any)
		next, found := properties[name].(map[string]any)
		if !found {
			t.Fatalf("openapi.yaml の %s に %s が無い", schema, path)
		}
		node = next
	}
	return resolve(node)
}

// openAPILimit は、スキーマの上限や下限（maxLength、maxItems、minimum、maximum）の値を返す。
func openAPILimit(t *testing.T, spec map[string]any, schema, path, keyword string) int64 {
	t.Helper()
	return schemaLimit(t, openAPIProperty(t, spec, schema, path), schema+"."+path, keyword)
}

// schemaLimit は、ひとつのスキーマの上限や下限の値を返す。location はエラーの文に出す場所である。
func schemaLimit(t *testing.T, node map[string]any, location, keyword string) int64 {
	t.Helper()
	switch value := node[keyword].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	default:
		t.Fatalf("openapi.yaml の %s に整数の %s が無い（%#v）", location, keyword, value)
		return 0
	}
}

// 上限の正本は Go の定数で、openapi.yaml はその写しである。画面の送る前の検査と
// 応答の検査は openapi.yaml から生成するので、ここが揃っていれば画面も揃う。
// Go だけを変えると、画面がサーバーより厳しくなるか、サーバーの返す値を画面が
// 断るようになる。
func TestAPILimitsMatchTheGoLimits(t *testing.T) {
	spec := openAPISpec(t)
	for _, limit := range []struct {
		schema, path, keyword string
		want                  int64
	}{
		{"SyncPushRequest", "message", "maxLength", remotesync.MaxCommitMessageRunes},
		{"SyncPushDraft", "message", "maxLength", remotesync.MaxCommitMessageRunes},
		{"SyncHistoryRevision", "message", "maxLength", remotesync.MaxCommitMessageRunes},
		{"SyncPushDraft", "added", "maximum", remotesync.MaxEntries},
		{"SyncPushDraft", "modified", "maximum", remotesync.MaxEntries},
		{"SyncPushDraft", "removed", "maximum", remotesync.MaxEntries},
		{"SyncHistoryRevision", "fileCount", "maximum", remotesync.MaxEntries},

		{"Metadata", "shortcutPresets", "maxItems", validate.MaxShortcutPresets},
		{"ShortcutPreset", "name", "maxLength", validate.MaxShortcutPresetNameLength},
		{"ShortcutPreset", "bindings.palette", "maxItems", validate.MaxShortcutKeysPerAction},

		{"WorkspaceDefinition", "name", "maxLength", workspace.MaxNameRunes},
		{"TerminalWorkspace", "name", "maxLength", workspace.MaxNameRunes},
		{"WorkspaceSplit", "ratio", "minimum", workspace.MinSplitRatio},
		{"WorkspaceSplit", "ratio", "maximum", workspace.MaxSplitRatio},

		{"FileTransferSettings", "maxConcurrent", "maximum", sftp.MaxTransferConcurrency},
		{"FileTransferSettings", "largeFileThresholdBytes", "minimum", sftp.MinLargeFileThreshold},
		{"FileTransferSettings", "largeFileThresholdBytes", "maximum", sftp.MaxLargeFileThreshold},
		{"FileTransferSettings", "largeFileParallelism", "maximum", sftp.MaxLargeFileParallelism},
		{"FileTransferSettings", "largeFileChunkBytes", "minimum", sftp.MinLargeFileChunkBytes},
		{"FileTransferSettings", "largeFileChunkBytes", "maximum", sftp.MaxLargeFileChunkBytes},
		{"SFTPTransferSettingsRequest", "maxConcurrent", "maximum", sftp.MaxTransferConcurrency},
		{"SFTPTransferSettingsRequest", "largeFileThresholdBytes", "minimum", sftp.MinLargeFileThreshold},
		{"SFTPTransferSettingsRequest", "largeFileThresholdBytes", "maximum", sftp.MaxLargeFileThreshold},
		{"SFTPTransferSettingsRequest", "largeFileParallelism", "maximum", sftp.MaxLargeFileParallelism},
		{"SFTPTransferSettingsRequest", "largeFileChunkBytes", "minimum", sftp.MinLargeFileChunkBytes},
		{"SFTPTransferSettingsRequest", "largeFileChunkBytes", "maximum", sftp.MaxLargeFileChunkBytes},
	} {
		if got := openAPILimit(t, spec, limit.schema, limit.path, limit.keyword); got != limit.want {
			t.Errorf("openapi.yaml の %s.%s の %s = %d、Go の上限は %d", limit.schema, limit.path, limit.keyword, got, limit.want)
		}
	}
}

// alias の長さの上限は validate.MaxAliasLength ひとつである。API のどこかだけ長くすると、
// サーバーが受け付けない alias を画面が送れてしまう。
func TestEveryAPIAliasHasTheAliasLengthLimit(t *testing.T) {
	spec := openAPISpec(t)
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	for schemaName, schema := range schemas {
		properties, _ := schema.(map[string]any)["properties"].(map[string]any)
		for propertyName, property := range properties {
			if propertyName != "alias" && !strings.HasSuffix(propertyName, "Alias") {
				continue
			}
			limit, limited := property.(map[string]any)["maxLength"]
			if limited && limit != validate.MaxAliasLength {
				t.Errorf("openapi.yaml の %s.%s の maxLength = %v、validate.MaxAliasLength は %d",
					schemaName, propertyName, limit, validate.MaxAliasLength)
			}
		}
	}
}

// VPN プロファイルの上限は、Go の検査と画面の検査が共有する表
// （internal/vpn/testdata/profile-cases.json）の limit に書いてある。その値が API の
// VPNProfile と同じであることを確かめる。
func TestAPIVPNProfileLimitsMatchTheSharedCases(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "vpn", "testdata", "profile-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Name   string `json:"name"`
			Field  string `json:"field"`
			Reason string `json:"reason"`
			Limit  int64  `json:"limit"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(body, &corpus); err != nil {
		t.Fatal(err)
	}
	keywords := map[string]string{"too_long": "maxLength", "too_many": "maxItems"}
	spec := openAPISpec(t)
	checked := 0
	for _, testCase := range corpus.Cases {
		keyword, limited := keywords[testCase.Reason]
		if !limited {
			continue
		}
		checked++
		property := openAPIProperty(t, spec, "VPNProfile", testCase.Field)
		location := "VPNProfile." + testCase.Field
		// 配列の field の too_long（wireguard.servers など）は、配列ではなく要素ひとつの長さの上限である。
		if items, isArray := property["items"].(map[string]any); isArray && keyword == "maxLength" {
			property, location = items, location+"[]"
		}
		if got := schemaLimit(t, property, location, keyword); got != testCase.Limit {
			t.Errorf("%s: openapi.yaml の %s の %s = %d、共有の表の上限は %d", testCase.Name, location, keyword, got, testCase.Limit)
		}
	}
	if checked == 0 {
		t.Fatal("共有の表に上限の例が無い。読んでいる表が違う")
	}
}
