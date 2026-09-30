package application

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// testPeerPublicKey は、項目の形の WireGuard のプロファイルの相手の公開鍵である。44 字の base64 で、
// 値そのものに意味は無い。
const testPeerPublicKey = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA="

// versionSevenMetadata は、v0.40.0（schema 7）が書いた、項目の形の WireGuard のプロファイルを
// 持つ文書である。
const versionSevenMetadata = `{"schemaVersion":7,` +
	`"vpnProfiles":[{"name":"lab","backend":"wireguard","dns":["10.9.9.53"],` +
	`"wireguard":{"server":"vpn.example.jp:51820",` +
	`"peerPublicKey":"` + testPeerPublicKey + `","address":"10.9.9.2/32"}},` +
	`{"name":"office","backend":"l2tp_ipsec","l2tp":{"server":"vpn.example.jp","username":"user"}}],` +
	`"hosts":[{"identity":{"path":"config","alias":"lab"},"vpn":"lab"}]}`

// v7 までの項目の形の WireGuard のプロファイルは、項目のまま読み、一覧に出すサーバーを補う。
// 設定ファイルを組み立てるには Vault の秘密鍵が要るので、metadata だけでは移さない。
func TestVersionSevenWireGuardProfilesKeepTheirFields(t *testing.T) {
	migrated, err := DecodeMetadata([]byte(versionSevenMetadata))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}
	stored := migrated.VPNProfiles[0]
	fields, found := stored.WireGuardFields()
	want := vpn.WireGuardFields{
		Server: "vpn.example.jp:51820", PeerPublicKey: testPeerPublicKey, Address: "10.9.9.2/32", DNS: []string{"10.9.9.53"},
	}
	if !found || fields.Server != want.Server || fields.PeerPublicKey != want.PeerPublicKey ||
		fields.Address != want.Address || !slices.Equal(fields.DNS, want.DNS) {
		t.Fatalf("WireGuardFields = %+v, %v", fields, found)
	}
	if !slices.Equal(stored.WireGuard.Servers, []string{"vpn.example.jp"}) {
		t.Fatalf("servers = %v", stored.WireGuard.Servers)
	}
	if _, err := stored.Profile(); err != nil {
		t.Fatalf("Profile = %v", err)
	}
	if _, found := migrated.VPNProfiles[1].WireGuardFields(); found || migrated.VPNProfiles[1].WireGuard != nil {
		t.Fatalf("ほかの方式のプロファイルに触れた: %+v", migrated.VPNProfiles[1])
	}
	encoded, err := EncodeMetadata(migrated)
	if err != nil {
		t.Fatalf("EncodeMetadata = %v", err)
	}
	for _, kept := range []string{`"schemaVersion": 9`, `"server": "vpn.example.jp:51820"`, testPeerPublicKey, `"servers"`} {
		if !strings.Contains(string(encoded), kept) {
			t.Fatalf("書き直した文書に %s が無い:\n%s", kept, encoded)
		}
	}
}

// 項目から設定ファイルを組み立てられないプロファイルは、metadata の検査で断る。経路を起動する
// まで気づかないままにしない。
func TestWireGuardFieldsThatCannotBecomeAConfigAreRefused(t *testing.T) {
	broken := strings.Replace(versionSevenMetadata, `"address":"10.9.9.2/32"`, `"address":"10.9.9"`, 1)
	metadata, err := DecodeMetadata([]byte(broken))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}

	if err := ValidateMetadata(metadata); !errors.Is(err, ErrMetadataVPN) {
		t.Fatalf("ValidateMetadata = %v, want %v", err, ErrMetadataVPN)
	}
}

// 保存し直したプロファイルは、項目を持たない。設定ファイルは Vault に書く。
func TestASavedWireGuardProfileDropsItsFields(t *testing.T) {
	metadata, err := DecodeMetadata([]byte(versionSevenMetadata))
	if err != nil {
		t.Fatal(err)
	}
	service := serviceWithVPNMetadata(t, metadata)
	profile, err := service.StoredVPNProfile("lab")
	if err != nil {
		t.Fatalf("StoredVPNProfile = %v", err)
	}

	change, err := service.PlanVPNProfileUpdate(profile)
	if err != nil {
		t.Fatalf("PlanVPNProfileUpdate = %v", err)
	}
	if _, err := service.CommitVPNProfileChange(change, nil); err != nil {
		t.Fatalf("CommitVPNProfileChange = %v", err)
	}

	saved, err := service.StoredVPNProfile("lab")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := saved.WireGuardFields(); found || !slices.Equal(saved.WireGuard.Servers, []string{"vpn.example.jp"}) {
		t.Fatalf("saved = %+v", saved.WireGuard)
	}
}
