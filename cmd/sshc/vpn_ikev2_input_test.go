package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// testCACertificate は、ファイルから読む値として使う PEM である。形は engine が確かめる。
const testCACertificate = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"

func savedIKEv2Profile() application.VPNProfile {
	return application.VPNProfile{
		Name: "office", Backend: vpn.IKEv2,
		IKEv2: &application.IKEv2Profile{
			Server: "vpn.example.jp", Authentication: vpn.IKEv2AuthenticationEAP, Identity: "fixture",
			CACertificate: testCACertificate,
		},
	}
}

// writeCACertificate は、CA の証明書のファイルを作り、その場所を返す。
func writeCACertificate(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// EAP では、ユーザー名とパスワードを読み、CA の証明書はファイルから読む。
func TestAddingAnIKEv2EAPProfileReadsTheCACertificateFromAFile(t *testing.T) {
	path := writeCACertificate(t, testCACertificate)
	p := profilePrompter(t, "ikev2\n\nvpn.example.jp\n\nfixture\n\n"+path+"\n\n\n", "password")

	input, err := readVPNProfile(p, "office", nil)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	want := application.IKEv2Profile{
		Server: "vpn.example.jp", Authentication: vpn.IKEv2AuthenticationEAP, Identity: "fixture",
		CACertificate: testCACertificate,
	}
	if input.profile.IKEv2 == nil || *input.profile.IKEv2 != want {
		t.Fatalf("profile = %+v", input.profile.IKEv2)
	}
	if len(input.secrets) != 1 || input.secrets[0].name != vpn.SecretKeyIKEv2Password {
		t.Fatalf("secrets = %+v", input.secrets)
	}
}

// 事前共有鍵では、CA の証明書を尋ねず、事前共有鍵を読む。
func TestAddingAnIKEv2PSKProfileAsksForThePreSharedKey(t *testing.T) {
	p := profilePrompter(t, "ikev2\n\nvpn.example.jp\npsk\nbranch@example.jp\nvpn.example.jp\n\n\n", "shared-secret")

	input, err := readVPNProfile(p, "branch", nil)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	want := application.IKEv2Profile{
		Server: "vpn.example.jp", Authentication: vpn.IKEv2AuthenticationPSK, Identity: "branch@example.jp",
		ServerIdentity: "vpn.example.jp",
	}
	if input.profile.IKEv2 == nil || *input.profile.IKEv2 != want {
		t.Fatalf("profile = %+v", input.profile.IKEv2)
	}
	if len(input.secrets) != 1 || input.secrets[0].name != vpn.SecretKeyIKEv2PSK ||
		string(input.secrets[0].value) != "shared-secret" {
		t.Fatalf("secrets = %+v", input.secrets)
	}
}

// 編集では、空欄なら保存済みの CA の証明書のまま、- なら消して公的な認証局で確かめる。
func TestEditingKeepsOrClearsTheSavedCACertificate(t *testing.T) {
	for _, test := range []struct {
		answer string
		want   string
	}{
		{"", testCACertificate},
		{clearWord, ""},
	} {
		p := profilePrompter(t, "\n\n\n\n\n\n"+test.answer+"\n\n\n", "")
		p.editing = true
		saved := savedIKEv2Profile()

		input, err := readVPNProfile(p, "office", &saved)

		if err != nil {
			t.Fatalf("readVPNProfile(%q) = %v", test.answer, err)
		}
		if input.profile.IKEv2.CACertificate != test.want || len(input.secrets) != 0 {
			t.Fatalf("answer %q: profile = %+v, secrets = %+v", test.answer, input.profile.IKEv2, input.secrets)
		}
	}
}

// 認証の方式を変えたら、保存済みのパスワードでは代わりにならない。事前共有鍵を求める。
func TestEditingToAnotherAuthenticationNeedsItsSecret(t *testing.T) {
	p := profilePrompter(t, "\n\n\npsk\n\n\n\n\n", "")
	p.editing = true
	saved := savedIKEv2Profile()

	if _, err := readVPNProfile(p, "office", &saved); !errors.Is(err, errVPNInputMissing) {
		t.Fatalf("readVPNProfile = %v, want errVPNInputMissing", err)
	}
}

// 送っても断られる大きさのファイルは、送る前に断る。
func TestAnOversizedCACertificateFileIsRefused(t *testing.T) {
	path := writeCACertificate(t, strings.Repeat("A", maxCACertificateFileBytes+1))
	p := profilePrompter(t, "ikev2\n\nvpn.example.jp\n\nfixture\n\n"+path+"\n\n\n", "password")

	if _, err := readVPNProfile(p, "office", nil); !errors.Is(err, errVPNInputCACertificateTooLarge) {
		t.Fatalf("readVPNProfile = %v, want errVPNInputCACertificateTooLarge", err)
	}
}
