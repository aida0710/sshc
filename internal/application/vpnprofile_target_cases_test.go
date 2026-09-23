package application

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// targetCasesPath は、Go と画面の接続先の照合が共有する表である。
var targetCasesPath = filepath.Join("..", "vpn", "testdata", "target-cases.json")

type targetCase struct {
	Name    string `json:"name"`
	Target  string `json:"target"`
	Address string `json:"address"`
	Reaches bool   `json:"reaches"`
}

// 共有の表のどの組も、保存形式の接続先から読んだ照合で、表のとおりの答えになる。
// 画面は保存形式の文字列を持つので、読み方（parseEndpoint）まで含めて揃える。
func TestVPNTargetsFollowTheSharedMatchingTable(t *testing.T) {
	contents, err := os.ReadFile(targetCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var table struct {
		About []string     `json:"about"`
		Cases []targetCase `json:"cases"`
	}
	if err := decoder.Decode(&table); err != nil {
		t.Fatalf("%s: %v", targetCasesPath, err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("表が空である")
	}
	for _, test := range table.Cases {
		t.Run(test.Name, func(t *testing.T) {
			stored := VPNProfile{Target: test.Target}
			if got := stored.Reaches(test.Address); got != test.Reaches {
				t.Fatalf("Reaches(%q) on %q = %v, want %v", test.Address, test.Target, got, test.Reaches)
			}
		})
	}
}
