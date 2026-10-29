package application

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"sshc/internal/knownhosts"
)

// 接続先ごとの名前は、ssh が鍵を探すのと同じ形になる。既定でないポートは [host]:port、
// HostKeyAlias はポート無しのその名前で、ワイルドカードの Host は接続先ではない。
func TestKnownHostNamesAreTheNamesSSHLooksTheKeysUpUnder(t *testing.T) {
	service, workspace := newTestService(t)
	config := "Host web\n\tHostName web.example.com\n\n" +
		"Host db\n\tHostName db.example.com\n\tPort 2222\n\n" +
		"Host lab\n\tHostName 10.0.0.5\n\tPort 2200\n\tHostKeyAlias Lab-Box\n\n" +
		"Host *.corp\n\tUser ops\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	names, err := service.KnownHostNames()
	if err != nil {
		t.Fatal(err)
	}
	want := []knownhosts.ConnectionName{
		{Alias: "web", Field: "web.example.com"},
		{Alias: "db", Field: "[db.example.com]:2222"},
		{Alias: "lab", Field: "lab-box"},
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %#v, want %#v", names, want)
	}
}
