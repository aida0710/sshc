package application

import (
	"errors"
	"slices"
	"testing"

	"sshc/internal/vpn"
)

func serviceWithVPNMetadata(t *testing.T, metadata Metadata) *Service {
	t.Helper()
	return serviceWithConfig(t, "Host lab\n  HostName 10.9.9.1\n", metadata)
}

func labProfile() VPNProfile {
	return VPNProfile{
		ID:        "0123456789abcdef0123456789abcdef",
		Name:      "lab",
		Backend:   vpn.WireGuard,
		WireGuard: &WireGuardProfile{Servers: []string{"vpn.example.jp"}},
	}
}

// 接続に結び付けたVPNプロファイルの名前を読める。alias は接続先の解決と同じく大文字と
// 小文字を区別するので、LAB にはどの Host ブロックも適用されず、VPN も付かない。
func TestAConnectionCarriesTheNameOfItsVPNProfile(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "lab"}, VPN: "lab",
	}}
	service := serviceWithVPNMetadata(t, metadata)

	name, err := service.ConnectionVPN("lab")
	if err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN = %q, %v", name, err)
	}
	if other, err := service.ConnectionVPN("LAB"); err != nil || other != "" {
		t.Fatalf("ConnectionVPN(LAB) = %q, %v", other, err)
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
	if profile.WireGuard == nil || !slices.Equal(profile.WireGuard.Servers, []string{"vpn.example.jp"}) {
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
	broken.DNS = []string{"dns.example.test"}
	metadata.VPNProfiles = []VPNProfile{broken}

	if _, err := EncodeMetadata(metadata); !errors.Is(err, ErrMetadataVPN) {
		t.Fatalf("EncodeMetadata = %v, want ErrMetadataVPN", err)
	}
}

// このバージョンが知らないbackendのプロファイルは保存しない。保存形式の変更は
// schemaVersion を上げて移行する。
func TestAProfileForAnUnknownBackendIsNotSaved(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{{Name: "future", Backend: "sstp"}}

	if _, err := EncodeMetadata(metadata); !errors.Is(err, ErrMetadataVPN) {
		t.Fatalf("EncodeMetadata = %v, want ErrMetadataVPN", err)
	}
}

// 方式と違う節を持つプロファイルは、作るときも置き換えるときも、その節を名指しして断る。
// 黙って落とすと、送った値が消えたことに気づけない。
func TestSavingAProfileWithTheSettingsOfAnotherBackendIsRefused(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	service := serviceWithVPNMetadata(t, metadata)
	withL2TP := labProfile()
	withL2TP.L2TP = &L2TPProfile{Server: "vpn.example.jp", Username: "user"}
	created := withL2TP
	created.Name = "office"

	_, createErr := service.PlanVPNProfileCreate(created)
	_, updateErr := service.PlanVPNProfileUpdate(withL2TP)

	for operation, err := range map[string]error{"create": createErr, "update": updateErr} {
		var refused *vpn.FieldError
		if !errors.Is(err, ErrMetadataVPN) || !errors.As(err, &refused) {
			t.Fatalf("%s = %v, want a field error", operation, err)
		}
		if refused.Field != "l2tp" || refused.Reason != vpn.ReasonUnexpected {
			t.Fatalf("%s refused %s/%s, want l2tp/unexpected", operation, refused.Field, refused.Reason)
		}
	}
}

// 一覧は名前の順に並ぶ。
func TestProfilesAreListedByName(t *testing.T) {
	metadata := NewMetadata()
	second, first := labProfile(), labProfile()
	second.Name, first.Name = "zeta", "alpha"
	second.ID, first.ID = "2e7a0000000000000000000000000000", "a1fa0000000000000000000000000000"
	metadata.VPNProfiles = []VPNProfile{second, first}
	service := serviceWithVPNMetadata(t, metadata)

	profiles, err := service.VPNProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 2 || profiles[0].Name != "alpha" || profiles[1].Name != "zeta" {
		t.Fatalf("profiles = %+v", profiles)
	}
}
