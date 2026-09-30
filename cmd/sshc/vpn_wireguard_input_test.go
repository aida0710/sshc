package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/vpn"
)

// テストの鍵である。どれも 44 字の base64 で、値そのものに意味は無い。
const (
	testWireGuardPublicKey    = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA="
	testWireGuardPresharedKey = "dDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDA="
)

// testWireGuardConfig は、鍵をそのまま書いた設定ファイルである。
var testWireGuardConfig = strings.Join([]string{
	"[Interface]",
	"PrivateKey = " + testVPNKey,
	"Address = 10.64.1.2/32",
	"DNS = 10.64.0.1",
	"",
	"[Peer]",
	"PublicKey = " + testWireGuardPublicKey,
	"PresharedKey = " + testWireGuardPresharedKey,
	"Endpoint = vpn.example.jp:51820",
	"AllowedIPs = 10.64.0.0/16",
	"",
}, "\n")

// 作成では、設定ファイルを鍵を含む本文のままシークレットとして送り、Endpoint のサーバーを
// 設定として送る。DNS は設定ファイルの DNS を使う。
func TestAddingAWireGuardProfileSendsTheConfigAsASecret(t *testing.T) {
	path := writeVPNConfigFile(t, testWireGuardConfig)
	p := profilePrompter(t, "wireguard\n"+path+"\n")

	input, err := readVPNProfile(p, "provider", nil)
	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	defer input.forget()

	if strings.Join(input.profile.WireGuard.Servers, ",") != "vpn.example.jp" ||
		strings.Join(input.profile.DNS, ",") != "10.64.0.1" {
		t.Fatalf("profile = %+v, dns = %v", input.profile.WireGuard, input.profile.DNS)
	}
	payload, err := buildVPNProfilePayload(input.profile, input.secrets)
	if err != nil {
		t.Fatalf("buildVPNProfilePayload = %v", err)
	}
	var decoded struct {
		Profile application.VPNProfile `json:"profile"`
		Secrets vpn.SecretsDocument    `json:"secrets"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload = %s: %v", payload, err)
	}
	if decoded.Secrets.WireGuardConfig != testWireGuardConfig {
		t.Fatalf("secrets = %+v", decoded.Secrets)
	}
	if encoded, _ := json.Marshal(decoded.Profile); strings.Contains(string(encoded), testVPNKey) ||
		strings.Contains(string(encoded), testWireGuardPresharedKey) {
		t.Fatalf("設定に鍵が入った: %s", encoded)
	}
}

// 編集では、空欄なら保存済みの設定ファイルのまま、シークレットも送らない。保存済みの設定
// ファイルは取り出さず、見せもしない。
func TestEditingAWireGuardProfileWithABlankPathKeepsEverything(t *testing.T) {
	saved := application.VPNProfile{
		Name: "provider", Backend: vpn.WireGuard, DNS: []string{"10.64.0.1"},
		WireGuard: &application.WireGuardProfile{Servers: []string{"vpn.example.jp"}},
	}
	p := profilePrompter(t, "\n\n")
	p.editing = true

	input, err := readVPNProfile(p, "provider", &saved)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if strings.Join(input.profile.WireGuard.Servers, ",") != "vpn.example.jp" || len(input.secrets) != 0 ||
		strings.Join(input.profile.DNS, ",") != "10.64.0.1" {
		t.Fatalf("input = %+v", input)
	}
	if shown := readPrompt(t, p); !strings.Contains(shown, "blank keeps the saved file") {
		t.Fatalf("空欄の意味を案内していない: %q", shown)
	}
}

// v0.40.0 までの項目の形のプロファイルも、空欄なら保存済みのまま送る。engine が項目と
// 秘密鍵から設定ファイルを組み立てて保存し直す。
func TestEditingAFieldsProfileWithABlankPathSendsItsServers(t *testing.T) {
	saved := application.VPNProfile{
		Name: "provider", Backend: vpn.WireGuard,
		WireGuard: &application.WireGuardProfile{
			Servers: []string{"vpn.example.jp"}, Server: "vpn.example.jp:51820",
			PeerPublicKey: testWireGuardPublicKey, Address: "10.64.1.2/32",
		},
	}
	p := profilePrompter(t, "\n\n")
	p.editing = true

	input, err := readVPNProfile(p, "provider", &saved)

	if err != nil {
		t.Fatalf("readVPNProfile = %v", err)
	}
	if strings.Join(input.profile.WireGuard.Servers, ",") != "vpn.example.jp" || input.profile.WireGuard.Server != "" {
		t.Fatalf("wireguard = %+v", input.profile.WireGuard)
	}
}

// readPrompt は、Prompter がターミナルへ書いた案内を読む。
func readPrompt(t *testing.T, p vpnProfilePrompter) string {
	t.Helper()
	shown, err := os.ReadFile(p.prompt.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(shown)
}

// 使えない項目を含む設定ファイルは、engine へ送る前に、行と項目を添えて断る。
func TestAWireGuardConfigWithACommandIsNotSent(t *testing.T) {
	path := writeVPNConfigFile(t, strings.Replace(testWireGuardConfig, "DNS = 10.64.0.1", "PostUp = echo up", 1))
	p := profilePrompter(t, "wireguard\n"+path+"\n")

	_, err := readVPNProfile(p, "provider", nil)

	var input *vpnInputError
	if !errors.As(err, &input) || !strings.Contains(input.sentence, `Line 4: "PostUp"`) ||
		strings.Contains(input.sentence, testVPNKey) {
		t.Fatalf("readVPNProfile = %v", err)
	}
}
