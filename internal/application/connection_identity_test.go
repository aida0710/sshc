package application

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/textencoding"
)

// serviceWithConfig は、~/.ssh/config に config を書き、metadata を置いた Service を作る。
func serviceWithConfig(t *testing.T, config string, metadata Metadata) *Service {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := NewService(workspace, manager)
	change, err := service.metadata.Change(metadata, storage.Precondition{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(storage.Request{Operation: "test", Changes: []storage.Change{change}}); err != nil {
		t.Fatal(err)
	}
	return service
}

// 大文字と小文字だけが違う Host ブロックが並んでも、接続先・VPN・文字コードは同じブロック
// から決まる。VPN を付け直すのも、そのブロックである。
func TestBlocksDifferingOnlyInCaseKeepTheirOwnVPNAndEncoding(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "WEB"}, VPN: "lab", Encoding: string(textencoding.ShiftJIS),
	}}
	service := serviceWithConfig(t, "Host web\n  HostName plain.example\n\nHost WEB\n  HostName other.example\n", metadata)

	resolved, err := service.ResolveConnection("WEB")
	if err != nil || resolved.First("hostname") != "other.example" {
		t.Fatalf("ResolveConnection(WEB) = %q, %v", resolved.First("hostname"), err)
	}
	if name, err := service.ConnectionVPN("WEB"); err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN(WEB) = %q, %v", name, err)
	}
	if encoding, err := service.ConnectionEncoding("WEB"); err != nil || encoding != textencoding.ShiftJIS {
		t.Fatalf("ConnectionEncoding(WEB) = %q, %v", encoding, err)
	}
	if name, err := service.ConnectionVPN("web"); err != nil || name != "" {
		t.Fatalf("ConnectionVPN(web) = %q, %v", name, err)
	}

	if _, err := service.SetConnectionVPN("web", "lab"); err != nil {
		t.Fatal(err)
	}
	bindings, err := service.VPNBindings()
	if err != nil || len(bindings["lab"]) != 2 {
		t.Fatalf("VPNBindings = %v, %v", bindings, err)
	}
}

// 別名で接続しても、その別名を宣言しているブロックの設定が付く。
func TestAnotherAliasOfTheBlockCarriesItsVPN(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{Identity: HostIdentity{Path: "config", Alias: "web"}, VPN: "lab"}}
	service := serviceWithConfig(t, "Host web web-alt\n  HostName 10.9.9.1\n", metadata)

	if name, err := service.ConnectionVPN("web-alt"); err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN(web-alt) = %q, %v", name, err)
	}
}

// 否定のパターンで alias を除くブロックの設定は付けない。
func TestABlockThatExcludesTheAliasDoesNotLendItsVPN(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{Identity: HostIdentity{Path: "config", Alias: "web"}, VPN: "lab"}}
	service := serviceWithConfig(t, "Host web !web\n  HostName 10.9.9.1\n", metadata)

	if name, err := service.ConnectionVPN("web"); err != nil || name != "" {
		t.Fatalf("ConnectionVPN(web) = %q, %v", name, err)
	}
}

// primaryAliasTakenByAnEarlierBlock は、後ろのブロックの primary alias（web）を、前の
// ブロックが先に宣言している設定である。web への接続は前のブロックを、b への接続は
// 後ろのブロックを使う。
const primaryAliasTakenByAnEarlierBlock = "Host a web\n  HostName 10.0.0.1\n\nHost web b\n  HostName 10.0.0.2\n"

// alias で VPN を付けると、その alias の接続が使うブロックに付く。画面はブロックを
// identity で書くので、alias で付けるのは CLI の sshc vpn bind である。
func TestBindingAVPNByAliasFollowsTheBlockTheConnectionUses(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	service := serviceWithConfig(t, primaryAliasTakenByAnEarlierBlock, metadata)

	if _, err := service.SetConnectionVPN("web", "lab"); err != nil {
		t.Fatal(err)
	}

	if name, err := service.ConnectionVPN("web"); err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN(web) = %q, %v", name, err)
	}
	if name, err := service.ConnectionVPN("b"); err != nil || name != "" {
		t.Fatalf("ConnectionVPN(b) = %q, %v; want the later block left alone", name, err)
	}
}

// primary alias を前のブロックに取られたブロックでも、そのブロックへ接続する alias で
// 検出した OS は残り、一覧にも出る。
func TestTheDetectedOSOfABlockWhosePrimaryAliasIsTakenIsKept(t *testing.T) {
	service := serviceWithConfig(t, primaryAliasTakenByAnEarlierBlock, NewMetadata())
	for _, connection := range []struct{ alias, os string }{{"b", "ubuntu"}, {"web", "debian"}} {
		target, err := sshclient.NewTarget(connection.alias, service.ResolveConnection, service.workspace.Home())
		if err != nil {
			t.Fatal(err)
		}
		observed := service.ObserveConnectionOS(target)
		if observed == nil {
			t.Fatalf("%s: no observer", connection.alias)
		}
		observed(connection.os)
	}

	overview, err := service.Overview()
	if err != nil {
		t.Fatal(err)
	}
	detected := map[HostIdentity]string{}
	for _, host := range overview.Metadata.Hosts {
		detected[host.Identity] = host.DetectedOS
	}
	want := map[HostIdentity]string{{Path: "config", Alias: "web"}: "ubuntu", {Path: "config", Alias: "a"}: "debian"}
	if !reflect.DeepEqual(detected, want) {
		t.Fatalf("detected = %v, want %v", detected, want)
	}
}
