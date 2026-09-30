package vpnprofile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// secretCasesPath は、画面の検査（web/src/vpn/vpnSecretRules.test.ts）と共有する表である。
var secretCasesPath = filepath.Join("..", "vpn", "testdata", "secret-cases.json")

type secretCase struct {
	Name    string                 `json:"name"`
	Profile application.VPNProfile `json:"profile"`
	Secrets vpn.SecretsDocument    `json:"secrets"`
	Stored  vpn.SecretsDocument    `json:"stored"`
	Field   string                 `json:"field"`
	Reason  string                 `json:"reason"`
	Limit   int                    `json:"limit"`
}

// 保存の経路は、共有の表のとおりにシークレットを受け取り、断る。保存済みの値に送った値を
// 重ねてから確かめる（Update と同じ順番）。
func TestSecretsFollowTheSharedValidationTable(t *testing.T) {
	contents, err := os.ReadFile(secretCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []secretCase `json:"cases"`
	}
	if err := json.Unmarshal(contents, &table); err != nil {
		t.Fatal(err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("the shared table has no cases")
	}
	for _, test := range table.Cases {
		t.Run(test.Name, func(t *testing.T) {
			merged := overlaySecrets(test.Stored, test.Secrets)

			_, err := encodeOwnSecrets(secretsToWrite{profile: test.Profile, secrets: merged.Secrets()})

			if test.Field == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			var refused *vpn.FieldError
			if !errors.As(err, &refused) {
				t.Fatalf("err = %v, want %s/%s", err, test.Field, test.Reason)
			}
			if refused.Field != test.Field || string(refused.Reason) != test.Reason || refused.Limit != test.Limit {
				t.Fatalf("refused %s/%s (limit %d), want %s/%s (limit %d)",
					refused.Field, refused.Reason, refused.Limit, test.Field, test.Reason, test.Limit)
			}
		})
	}
}
