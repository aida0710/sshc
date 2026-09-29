package vpn

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// wireGuardConfigCase は、testdata/wireguard-config-cases.json の1件である。画面の検査も
// 同じ表でテストする。
type wireGuardConfigCase struct {
	Name      string   `json:"name"`
	Config    []string `json:"config"`
	Reason    Reason   `json:"reason"`
	Line      int      `json:"line"`
	Directive string   `json:"directive"`
	Limit     int      `json:"limit"`
	DNS       []string `json:"dns"`
	Addresses []string `json:"addresses"`
	Servers   []string `json:"servers"`
}

func loadWireGuardConfigCases(t *testing.T) []wireGuardConfigCase {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "wireguard-config-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []wireGuardConfigCase `json:"cases"`
	}
	if err := json.Unmarshal(contents, &table); err != nil {
		t.Fatal(err)
	}
	return table.Cases
}

// 設定ファイルの規則は、画面と同じ表で確かめる。
func TestWireGuardConfigsFollowTheSharedTable(t *testing.T) {
	for _, test := range loadWireGuardConfigCases(t) {
		t.Run(test.Name, func(t *testing.T) {
			config, err := ParseWireGuardConfig(joinConfigLines(test.Config))
			if test.Reason != "" {
				var refused *ConfigLineError
				if !errors.As(err, &refused) {
					t.Fatalf("ParseWireGuardConfig = %v, want a refusal", err)
				}
				if refused.Reason != test.Reason || refused.Line != test.Line || refused.Directive != test.Directive ||
					refused.Limit != test.Limit || refused.Field != "secrets.wireguardConfig" {
					t.Fatalf("refusal = %s %s line %d %q limit %d, want %s line %d %q limit %d", refused.Field,
						refused.Reason, refused.Line, refused.Directive, refused.Limit,
						test.Reason, test.Line, test.Directive, test.Limit)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWireGuardConfig = %v", err)
			}
			addresses := make([]string, 0, len(config.Addresses))
			for _, address := range config.Addresses {
				addresses = append(addresses, address.String())
			}
			if !slices.Equal(addresses, test.Addresses) || !slices.Equal(config.DNS, nilIfEmpty(test.DNS)) ||
				!slices.Equal(config.Servers(), test.Servers) {
				t.Fatalf("addresses %v dns %v servers %v, want %v %v %v",
					addresses, config.DNS, config.Servers(), test.Addresses, test.DNS, test.Servers)
			}
		})
	}
}

func nilIfEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

// 読み取った鍵は、使い終わったら Forget で消せる。CLI は設定ファイルを送り終えたら消す。
func TestForgetClearsTheKeysThatWereRead(t *testing.T) {
	config, err := ParseWireGuardConfig(joinConfigLines(loadWireGuardConfigCases(t)[0].Config))
	if err != nil {
		t.Fatal(err)
	}
	privateKey, presharedKey := config.PrivateKey.Value, config.Peers[0].PresharedKey.Value
	if len(privateKey) == 0 || len(presharedKey) == 0 {
		t.Fatal("鍵を読み取っていない")
	}

	config.Forget()

	if strings.Trim(string(privateKey), "\x00") != "" || strings.Trim(string(presharedKey), "\x00") != "" {
		t.Fatal("Forget が鍵を消していない")
	}
}

// 長すぎる設定ファイルは読まない。
func TestAnOverlongWireGuardConfigIsRefused(t *testing.T) {
	_, err := ParseWireGuardConfig([]byte(strings.Repeat("#\n", MaxWireGuardConfigLength)))

	var refused *ConfigLineError
	if !errors.As(err, &refused) || refused.Reason != ReasonTooLong || refused.Limit != MaxWireGuardConfigLength {
		t.Fatalf("err = %v", err)
	}
}
