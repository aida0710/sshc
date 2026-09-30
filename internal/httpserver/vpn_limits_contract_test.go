package httpserver

import (
	"testing"

	"sshc/internal/vpn"
)

// API が受け付けるファイルの長さの上限は、engine が断る上限と同じである。片方だけを
// 変えると、API の検査が先に断るか、engine が API の約束より早く断る。
func TestTheAPILimitsOfVPNFilesAreTheEngineLimits(t *testing.T) {
	t.Parallel()
	document := readOpenAPIDocument(t)
	for _, test := range []struct {
		schema, property string
		limit            int
	}{
		{"IKEv2Profile", "caCertificate", vpn.MaxCACertificateLength},
		{"VPNSecrets", "wireguardConfig", vpn.MaxWireGuardConfigLength},
		{"VPNSecrets", "openvpnConfig", vpn.MaxOpenVPNConfigLength},
	} {
		properties, _ := document.Components.Schemas[test.schema]["properties"].(map[string]any)
		property, _ := properties[test.property].(map[string]any)
		if maxLength, _ := property["maxLength"].(int); maxLength != test.limit {
			t.Errorf("%s.%s maxLength = %v, want %d", test.schema, test.property, property["maxLength"], test.limit)
		}
	}
}
