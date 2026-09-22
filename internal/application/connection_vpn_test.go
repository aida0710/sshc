package application

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/storage"
	"sshc/internal/vpn"
)

func serviceWithVPNMetadata(t *testing.T, metadata Metadata) *Service {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Host lab\n  HostName 10.9.9.1\n"), 0o600); err != nil {
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

func labProfile() VPNProfile {
	return VPNProfile{
		Name:    "lab",
		Backend: string(vpn.WireGuard),
		Target:  "10.9.9.1:22",
		WireGuard: &WireGuardProfile{
			Server:        "vpn.example.jp:51820",
			PeerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=",
			Address:       "10.9.9.2/32",
		},
	}
}

// 接続に結び付けたVPNプロファイルの名前を読める。
func TestAConnectionCarriesTheNameOfItsVPNProfile(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "lab"}, VPN: "lab",
	}}
	service := serviceWithVPNMetadata(t, metadata)

	name, err := service.ConnectionVPN("LAB")
	if err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN = %q, %v", name, err)
	}
	unbound, err := service.ConnectionVPN("other")
	if err != nil || unbound != "" {
		t.Fatalf("ConnectionVPN(other) = %q, %v", unbound, err)
	}
}

// 保存した設定は、経路を作る側が使う形で読み出せる。
func TestAStoredProfileBecomesTheRouteDefinition(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	service := serviceWithVPNMetadata(t, metadata)

	profile, err := service.VPNProfile("lab")
	if err != nil {
		t.Fatalf("VPNProfile = %v", err)
	}
	if profile.Target.Host != "10.9.9.1" || profile.Target.Port != 22 {
		t.Fatalf("target = %+v", profile.Target)
	}
	if profile.WireGuard == nil || profile.WireGuard.Server.Port != 51820 {
		t.Fatalf("wireguard = %+v", profile.WireGuard)
	}
	listed, err := service.VPNProfiles()
	if err != nil || len(listed) != 1 || listed[0].Name != "lab" {
		t.Fatalf("VPNProfiles = %+v, %v", listed, err)
	}
}

// 無い名前は、繋ぐ前に理由が分かる。
func TestAMissingProfileIsRefusedByName(t *testing.T) {
	service := serviceWithVPNMetadata(t, NewMetadata())

	if _, err := service.VPNProfile("absent"); !errors.Is(err, ErrUnknownVPNProfile) {
		t.Fatalf("VPNProfile = %v, want ErrUnknownVPNProfile", err)
	}
}

// 保存の時点で形の壊れたプロファイルは書けない。
func TestAProfileThatCannotBecomeARouteIsNotSaved(t *testing.T) {
	metadata := NewMetadata()
	broken := labProfile()
	broken.Target = "example.test:22"
	metadata.VPNProfiles = []VPNProfile{broken}

	if _, err := EncodeMetadata(metadata); !errors.Is(err, ErrMetadataVPN) {
		t.Fatalf("EncodeMetadata = %v, want ErrMetadataVPN", err)
	}
}

// この版が知らないbackendのプロファイルは、保存を妨げない。
//
// プロファイルは端末のあいだで同期される。新しい版が書いたものを理由に、古い版で
// metadataを保存できなくしない。
func TestAProfileForAnUnknownBackendStillSaves(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{{Name: "future", Backend: "openvpn", Target: "10.9.9.1:22"}}

	if _, err := EncodeMetadata(metadata); err != nil {
		t.Fatalf("EncodeMetadata = %v", err)
	}
}
