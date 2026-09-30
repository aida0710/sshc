package application

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sshc/internal/vpn"
)

// versionEightSections は、v8 までにあった方式ごとの、受け付けられる節である。
var versionEightSections = map[vpn.BackendName]string{
	vpn.WireGuard:   `"wireguard":{"servers":["vpn.example.jp"]}`,
	vpn.L2TPIPsec:   `"l2tp":{"server":"vpn.example.jp","username":"fixture"}`,
	vpn.OpenConnect: `"openconnect":{"server":"vpn.example.jp","username":"fixture"}`,
	vpn.OpenVPN:     `"openvpn":{"servers":["vpn.example.jp"]}`,
	vpn.IKEv2:       `"ikev2":{"server":"vpn.example.jp","authentication":"eap-mschapv2","identity":"fixture"}`,
}

// metadataWithEverySection は、方式ごとにひとつ、v8 までのどの方式の節も持つプロファイルを
// 置いた metadata である。
func metadataWithEverySection(schemaVersion int) string {
	sections := make([]string, 0, len(versionEightSections))
	for _, section := range versionEightSections {
		sections = append(sections, section)
	}
	profiles := make([]string, 0, len(versionEightSections))
	for backend := range versionEightSections {
		profiles = append(profiles,
			fmt.Sprintf(`{"name":"%s","backend":"%s",%s}`, backend, backend, strings.Join(sections, ",")))
	}
	return fmt.Sprintf(`{"schemaVersion":%d,"vpnProfiles":[%s]}`, schemaVersion, strings.Join(profiles, ","))
}

// v8 からの移行は、どのプロファイルでも backend と違う節を外し、その方式の節だけを残す。
// 移行したあとの metadata はそのまま書ける。
func TestMigratingFromVersionEightKeepsOnlyTheSectionOfTheBackend(t *testing.T) {
	migrated, err := DecodeMetadata([]byte(metadataWithEverySection(8)))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}

	for _, stored := range migrated.VPNProfiles {
		present := map[vpn.BackendName]bool{
			vpn.WireGuard:   stored.WireGuard != nil,
			vpn.L2TPIPsec:   stored.L2TP != nil,
			vpn.OpenConnect: stored.OpenConnect != nil,
			vpn.OpenVPN:     stored.OpenVPN != nil,
			vpn.IKEv2:       stored.IKEv2 != nil,
		}
		for backend, found := range present {
			if found != (backend == stored.Backend) {
				t.Errorf("%s: %s の節が残っている = %v", stored.Name, backend, found)
			}
		}
	}
	if _, err := EncodeMetadata(migrated); err != nil {
		t.Fatalf("EncodeMetadata = %v", err)
	}
}

// v9 の metadata は移行しない。backend と違う節は、書くときの検査で断る。
func TestVersionNineMetadataKeepsTheSectionsOfOtherBackendsAndIsRefused(t *testing.T) {
	decoded, err := DecodeMetadata([]byte(metadataWithEverySection(9)))
	if err != nil {
		t.Fatalf("DecodeMetadata = %v", err)
	}

	if _, err := EncodeMetadata(decoded); !errors.Is(err, ErrMetadataVPN) {
		t.Fatalf("EncodeMetadata = %v, want ErrMetadataVPN", err)
	}
}
