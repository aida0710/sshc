package secret_test

import (
	"errors"
	"testing"

	"sshc/internal/secret"
)

// VPN の秘密は保存して読み出せる。
func TestVPNSecretsAreStoredAndReadBack(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	if err := service.SetVPNSecrets("tohoku", `{"wireguardPrivateKey":"fixture"}`); err != nil {
		t.Fatalf("SetVPNSecrets = %v", err)
	}

	stored, err := service.VPNSecrets("tohoku")
	if err != nil || stored != `{"wireguardPrivateKey":"fixture"}` {
		t.Fatalf("VPNSecrets = %q, %v", stored, err)
	}
	if err := service.RemoveVPNSecrets("tohoku"); err != nil {
		t.Fatalf("RemoveVPNSecrets = %v", err)
	}
	if _, err := service.VPNSecrets("tohoku"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("VPNSecrets after removal = %v", err)
	}
}

// VPN の秘密は、資格情報の画面と API の名前空間には現れない。
func TestVPNSecretsAreNotReachableThroughTheCredentialNamespaces(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetVPNSecrets("tohoku", `{"wireguardPrivateKey":"fixture"}`); err != nil {
		t.Fatal(err)
	}

	if _, err := service.Credential(secret.Kind("vpn"), "tohoku"); !errors.Is(err, secret.ErrUnknownKind) {
		t.Fatalf("Credential(vpn) = %v, want ErrUnknownKind", err)
	}
	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	for kind, names := range listed {
		if len(names) != 0 {
			t.Fatalf("資格情報の一覧に %s が %v として現れた", kind, names)
		}
	}
}
