package application

import (
	"strings"
	"testing"
)

// versionEightIKEv2Metadata は、v8 の metadata に、サーバーの ID を持つ IKEv2 の
// プロファイルを2つ置いたものである。
const versionEightIKEv2Metadata = `{"schemaVersion":8,"vpnProfiles":[` +
	`{"name":"office","backend":"ikev2","ikev2":{"server":"vpn.example.jp","authentication":"eap-mschapv2","identity":"fixture","serverIdentity":"*"}},` +
	`{"name":"lab","backend":"ikev2","ikev2":{"server":"vpn.example.jp","authentication":"eap-mschapv2","identity":"fixture","serverIdentity":"vpn.example.jp"}}]}`

// v8 からの移行は、どのサーバーにも一致するサーバーの ID を外し、ひとつのサーバーに
// 決まる ID は残す。移行したあとの metadata はそのまま書ける。
func TestMigratingFromVersionEightClearsServerIdentitiesThatMatchAnyServer(t *testing.T) {
	migrated, err := DecodeMetadata([]byte(versionEightIKEv2Metadata))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}

	if identity := migrated.VPNProfiles[0].IKEv2.ServerIdentity; identity != "" {
		t.Errorf("office のサーバーの ID = %q, want 空", identity)
	}
	if identity := migrated.VPNProfiles[1].IKEv2.ServerIdentity; identity != "vpn.example.jp" {
		t.Errorf("lab のサーバーの ID = %q, want vpn.example.jp", identity)
	}
	encoded, err := EncodeMetadata(migrated)
	if err != nil {
		t.Fatalf("EncodeMetadata = %v", err)
	}
	if !strings.Contains(string(encoded), `"schemaVersion": 9`) {
		t.Fatalf("encoded = %s", encoded)
	}
}

// v9 の metadata は移行しない。どのサーバーにも一致する ID は、書くときの検査で断る。
func TestVersionNineMetadataIsNotMigratedAgain(t *testing.T) {
	current := strings.Replace(versionEightIKEv2Metadata, `"schemaVersion":8`, `"schemaVersion":9`, 1)

	decoded, err := DecodeMetadata([]byte(current))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}

	if identity := decoded.VPNProfiles[0].IKEv2.ServerIdentity; identity != "*" {
		t.Fatalf("office のサーバーの ID = %q, want *", identity)
	}
	if _, err := EncodeMetadata(decoded); err == nil {
		t.Fatal("どのサーバーにも一致するサーバーの ID を書いた")
	}
}
