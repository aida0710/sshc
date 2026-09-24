package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// profilePrompter は、lines を入力し、secrets をシークレットの入力として読む Prompter を作る。
func profilePrompter(t *testing.T, lines string, secrets ...string) vpnProfilePrompter {
	t.Helper()
	answers := make([][]byte, 0, len(secrets))
	for _, secret := range secrets {
		answers = append(answers, []byte(secret))
	}
	prompt, err := os.CreateTemp(t.TempDir(), "prompt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prompt.Close() })
	return vpnProfilePrompter{
		ctx: context.Background(), stdin: writeTemporaryFile(t, lines), prompt: prompt,
		terminal: &fakePasswordTerminal{terminal: true, answers: answers},
	}
}

func savedL2TPProfile() application.VPNProfile {
	return application.VPNProfile{
		Name: "office", Backend: vpn.L2TPIPsec, DNS: []string{"10.9.9.53"},
		L2TP: &application.L2TPProfile{Server: "vpn.example.jp", Username: "fixture", IKE: "aes256-sha256-modp2048"},
	}
}

// 編集では、何も入力しなければ保存済みの値のまま、シークレットも送らない。
func TestEditingWithBlankAnswersKeepsEverything(t *testing.T) {
	p := profilePrompter(t, "\n\n\n\n\n\n", "", "")
	p.editing = true
	saved := savedL2TPProfile()

	input, err := readVPNProfile(p, "office", &saved)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if input.profile.Backend != vpn.L2TPIPsec || len(input.profile.DNS) != 1 || *input.profile.L2TP != *saved.L2TP {
		t.Fatalf("profile = %+v %+v", input.profile, input.profile.L2TP)
	}
	if len(input.secrets) != 0 {
		t.Fatalf("空欄のシークレットを送ろうとした: %+v", input.secrets)
	}
}

// 編集では、任意の項目は - で消せる。
func TestEditingCanClearAnOptionalSetting(t *testing.T) {
	p := profilePrompter(t, "\n-\n\n\n-\n\n", "new-password", "")
	p.editing = true
	saved := savedL2TPProfile()

	input, err := readVPNProfile(p, "office", &saved)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if input.profile.DNS != nil || input.profile.L2TP.IKE != "" {
		t.Fatalf("profile = %+v %+v", input.profile, input.profile.L2TP)
	}
	if len(input.secrets) != 1 || input.secrets[0].name != vpn.SecretKeyL2TPPassword {
		t.Fatalf("secrets = %+v", input.secrets)
	}
}

// 編集で方式を変えたら、新しい方式のシークレットを求める。前の方式のシークレットは使えない。
func TestEditingToAnotherBackendNeedsItsSecrets(t *testing.T) {
	p := profilePrompter(t, "wireguard\n\nvpn.example.jp:51820\nbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=\n\n", "")
	p.editing = true
	saved := savedL2TPProfile()

	if _, err := readVPNProfile(p, "office", &saved); !errors.Is(err, errVPNInputMissing) {
		t.Fatalf("readVPNProfile = %v, want errVPNInputMissing", err)
	}
}

// 作成では、シークレットは省略できない。
func TestAddingNeedsEverySecret(t *testing.T) {
	p := profilePrompter(t, "l2tp_ipsec\n\nvpn.example.jp\nfixture\n\n\n", "password", "")

	if _, err := readVPNProfile(p, "office", nil); !errors.Is(err, errVPNInputMissing) {
		t.Fatalf("readVPNProfile = %v, want errVPNInputMissing", err)
	}
}
