package vpn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// openVPNConfigCase は、testdata/openvpn-config-cases.json の1件である。画面の検査も
// 同じ表でテストする。
type openVPNConfigCase struct {
	Name            string   `json:"name"`
	Config          []string `json:"config"`
	Servers         []string `json:"servers"`
	AsksCredentials bool     `json:"asksCredentials"`
	Reason          Reason   `json:"reason"`
	Line            int      `json:"line"`
	Directive       string   `json:"directive"`
	Limit           int      `json:"limit"`
}

func loadOpenVPNConfigCases(t *testing.T) []openVPNConfigCase {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "openvpn-config-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []openVPNConfigCase `json:"cases"`
	}
	if err := json.Unmarshal(contents, &table); err != nil {
		t.Fatal(err)
	}
	return table.Cases
}

// joinConfigLines は、行の並びを設定ファイルにする。
func joinConfigLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// 設定ファイルの規則は、画面と同じ表で確かめる。
func TestOpenVPNConfigsFollowTheSharedTable(t *testing.T) {
	for _, test := range loadOpenVPNConfigCases(t) {
		t.Run(test.Name, func(t *testing.T) {
			summary, err := InspectOpenVPNConfig(joinConfigLines(test.Config))
			if test.Reason == "" {
				if err != nil {
					t.Fatalf("InspectOpenVPNConfig = %v", err)
				}
				if !slices.Equal(summary.Servers, test.Servers) || summary.AsksCredentials != test.AsksCredentials {
					t.Fatalf("summary = %+v, want servers %v and asksCredentials %v",
						summary, test.Servers, test.AsksCredentials)
				}
				return
			}
			var refused *ConfigLineError
			if !errors.As(err, &refused) {
				t.Fatalf("InspectOpenVPNConfig = %v, want a refusal", err)
			}
			if refused.Reason != test.Reason || refused.Line != test.Line || refused.Directive != test.Directive ||
				refused.Limit != test.Limit {
				t.Fatalf("refusal = %s line %d %q limit %d, want %s line %d %q limit %d",
					refused.Reason, refused.Line, refused.Directive, refused.Limit,
					test.Reason, test.Line, test.Directive, test.Limit)
			}
		})
	}
}

// 断った理由は、項目の誤りとして扱える。API の problem と CLI の文は、それを使う。
func TestARefusedDirectiveIsAFieldErrorOfTheConfig(t *testing.T) {
	_, err := InspectOpenVPNConfig([]byte("client\nremote vpn.example.jp\nup /bin/true\n"))

	var field *FieldError
	if !errors.As(err, &field) || field.Field != "secrets.openvpnConfig" || field.Reason != ReasonRunsCommand {
		t.Fatalf("err = %v, want a field error of secrets.openvpnConfig", err)
	}
	if !errors.Is(err, ErrSecrets) {
		t.Fatalf("err = %v, want ErrSecrets", err)
	}
}

// OpenVPN が持てる数より多い remote は断る。
func TestTooManyRemotesAreRefused(t *testing.T) {
	lines := []string{"client"}
	for index := range maxOpenVPNRemotes + 1 {
		lines = append(lines, fmt.Sprintf("remote vpn%d.example.jp", index))
	}

	_, err := InspectOpenVPNConfig(joinConfigLines(lines))

	var refused *ConfigLineError
	if !errors.As(err, &refused) || refused.Reason != ReasonTooMany || refused.Limit != maxOpenVPNRemotes ||
		refused.Line != maxOpenVPNRemotes+2 {
		t.Fatalf("err = %v, want too_many at line %d", err, maxOpenVPNRemotes+2)
	}
}

// 表にある断る指示は、どれも実際に断る。表の名前を綴り間違えると、断ったつもりの
// 指示が素通りする。
func TestEveryListedDirectiveIsRefused(t *testing.T) {
	for directive, reason := range refusedOpenVPNDirectives {
		_, err := InspectOpenVPNConfig([]byte("client\nremote vpn.example.jp\n" + directive + " x\n"))
		var refused *ConfigLineError
		if !errors.As(err, &refused) || refused.Reason != reason || refused.Directive != directive {
			t.Errorf("%s: err = %v, want %s", directive, err, reason)
		}
	}
}
