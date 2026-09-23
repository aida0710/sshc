package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/vpn"
)

// profileCasesPath は、Go と画面のフォームが共有する検査の表である。
var profileCasesPath = filepath.Join("..", "vpn", "testdata", "profile-cases.json")

type profileCase struct {
	Name    string     `json:"name"`
	Profile VPNProfile `json:"profile"`
	Field   string     `json:"field"`
	Reason  string     `json:"reason"`
	Limit   int        `json:"limit"`
}

// 共有の表のどの入力も、表に書いた項目と理由で断られるか、断られずに通る。
func TestVPNProfilesFollowTheSharedValidationTable(t *testing.T) {
	contents, err := os.ReadFile(profileCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	// 表に知らない項目があれば、どちらかの読み手が黙って読み落とす。
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var table struct {
		About []string      `json:"about"`
		Cases []profileCase `json:"cases"`
	}
	if err := decoder.Decode(&table); err != nil {
		t.Fatalf("%s: %v", profileCasesPath, err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("表が空である")
	}
	for _, test := range table.Cases {
		t.Run(test.Name, func(t *testing.T) {
			_, err := test.Profile.Profile()
			if test.Field == "" {
				if err != nil {
					t.Fatalf("Profile() = %v, want accepted", err)
				}
				return
			}
			var refused *vpn.FieldError
			if !errors.As(err, &refused) {
				t.Fatalf("Profile() = %v, want a field error on %s", err, test.Field)
			}
			if refused.Field != test.Field || string(refused.Reason) != test.Reason || refused.Limit != test.Limit {
				t.Fatalf("Profile() refused %s/%s (limit %d), want %s/%s (limit %d)",
					refused.Field, refused.Reason, refused.Limit, test.Field, test.Reason, test.Limit)
			}
		})
	}
}
