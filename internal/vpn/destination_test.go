package vpn

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// destinationCasesPath は、Go と画面が共有する、接続先の規則の表である。
var destinationCasesPath = filepath.Join("testdata", "destination-cases.json")

type destinationCase struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	DNS     bool   `json:"dns"`
	Reason  Reason `json:"reason"`
}

// 接続先は、プロファイルを付けた接続から来る。表のどの組も、表のとおりに通すか断る。
func TestDestinationsFollowTheSharedTable(t *testing.T) {
	contents, err := os.ReadFile(destinationCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var table struct {
		About []string          `json:"about"`
		Cases []destinationCase `json:"cases"`
	}
	if err := decoder.Decode(&table); err != nil {
		t.Fatalf("%s: %v", destinationCasesPath, err)
	}
	if len(table.Cases) == 0 {
		t.Fatal("表が空である")
	}
	for _, test := range table.Cases {
		t.Run(test.Name, func(t *testing.T) {
			profile := validProfile()
			if test.DNS {
				profile.DNS = []string{"10.9.9.53"}
			}
			destination, err := profile.Destination(test.Address)
			if test.Reason == "" {
				if err != nil {
					t.Fatalf("Destination(%q) = %v", test.Address, err)
				}
				if destination.Address() != test.Address {
					t.Fatalf("Destination(%q) = %v", test.Address, destination)
				}
				return
			}
			var failure *DestinationError
			if !errors.As(err, &failure) || failure.Reason != test.Reason || !errors.Is(err, ErrDestination) {
				t.Fatalf("Destination(%q) = %v, want %s", test.Address, err, test.Reason)
			}
		})
	}
}
