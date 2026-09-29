package vpn

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 中間 CA を含めて何枚でも貼れる。swanctl は1ファイルから1枚しか読まないので、
// 1枚ずつのファイルにして、どれも信頼する CA として指定する。
func TestEachCACertificateBecomesItsOwnFile(t *testing.T) {
	profile := ikev2EAPProfile()
	root, intermediate := newTestAuthority(t, "root"), newTestAuthority(t, "intermediate")
	profile.IKEv2.CACertificate = root.PEM + "\n" + intermediate.PEM

	documents, err := ikev2Documents(*profile.IKEv2, *ikev2Secrets().IKEv2)
	if err != nil {
		t.Fatalf("ikev2Documents = %v", err)
	}

	if documents["ca-1.pem"] != root.PEM || documents["ca-2.pem"] != intermediate.PEM {
		t.Fatalf("CA files = %q, %q", documents["ca-1.pem"], documents["ca-2.pem"])
	}
	if !strings.Contains(documents["swanctl.conf"], "cacerts = /run/sshc-vpn/ca-1.pem,/run/sshc-vpn/ca-2.pem\n") {
		t.Fatalf("swanctl.conf が CA を指していない:\n%s", documents["swanctl.conf"])
	}
}

// サーバーの ID は、指定しなければサーバーの名前にする。空のまま（%any）にすると、
// 信頼する認証局の証明書を持つ誰とでも繋いでしまう。
func TestTheServerIdentityDefaultsToTheServerName(t *testing.T) {
	profile := ikev2EAPProfile()
	configuration := ikev2SwanctlConfiguration(*profile.IKEv2, *ikev2Secrets().IKEv2, nil)
	if !strings.Contains(configuration, "    remote {\n            id = \"vpn.example.jp\"\n") {
		t.Fatalf("サーバーの名前がサーバーの ID になっていない:\n%s", configuration)
	}

	profile.IKEv2.ServerIdentity = "CN=vpn.example.jp, O=Example"
	configuration = ikev2SwanctlConfiguration(*profile.IKEv2, *ikev2Secrets().IKEv2, nil)
	if !strings.Contains(configuration, `id = "CN=vpn.example.jp, O=Example"`) {
		t.Fatalf("指定したサーバーの ID を使っていない:\n%s", configuration)
	}
}

// 相手のアドレスは設定に書かず、コンテナの中で名前解決した値で置き換える。
func TestTheIKEv2ServerAddressIsLeftForTheContainerToResolve(t *testing.T) {
	configuration := ikev2SwanctlConfiguration(*ikev2EAPProfile().IKEv2, *ikev2Secrets().IKEv2, nil)

	if !strings.Contains(configuration, "remote_addrs = "+serverAddressPlaceholder+"\n") {
		t.Fatalf("swanctl.conf が印を持たない:\n%s", configuration)
	}
}

// EAP では、サーバーを証明書で確かめ、こちらはユーザー名とパスワードで認証する。
func TestEAPVerifiesTheServerByCertificateAndSendsTheUsername(t *testing.T) {
	configuration := ikev2SwanctlConfiguration(*ikev2EAPProfile().IKEv2, *ikev2Secrets().IKEv2, nil)

	for _, want := range []string{
		"auth = eap-mschapv2\n", `eap_id = "vpn-user"`, "auth = pubkey\n",
		"eap-sshc-vpn {\n", "secret = 0x" + hex.EncodeToString([]byte(`p"a\ss`)) + "\n",
	} {
		if !strings.Contains(configuration, want) {
			t.Errorf("swanctl.conf に %q が無い:\n%s", want, configuration)
		}
	}
	for _, unwanted := range []string{"auth = psk", "cacerts", "ike-sshc-vpn", `p"a\ss`,
		hex.EncodeToString([]byte("shared-secret"))} {
		if strings.Contains(configuration, unwanted) {
			t.Errorf("swanctl.conf に %q がある:\n%s", unwanted, configuration)
		}
	}
}

// 事前共有鍵では、サーバーも同じ事前共有鍵で認証する。証明書は使わない。
func TestPSKAuthenticatesBothSidesWithThePreSharedKey(t *testing.T) {
	configuration := ikev2SwanctlConfiguration(*ikev2PSKProfile().IKEv2, *ikev2Secrets().IKEv2, nil)

	if strings.Count(configuration, "auth = psk\n") != 2 {
		t.Errorf("両側が事前共有鍵になっていない:\n%s", configuration)
	}
	for _, want := range []string{`id = "branch@example.jp"`, "ike-sshc-vpn {\n",
		"secret = 0x" + hex.EncodeToString([]byte("shared-secret")) + "\n"} {
		if !strings.Contains(configuration, want) {
			t.Errorf("swanctl.conf に %q が無い:\n%s", want, configuration)
		}
	}
	for _, unwanted := range []string{"pubkey", "eap", "shared-secret"} {
		if strings.Contains(configuration, unwanted) {
			t.Errorf("swanctl.conf に %q がある:\n%s", unwanted, configuration)
		}
	}
}

// トンネルは XFRM インターフェースに結び付け、strongSwan には経路も DNS も入れさせない。
func TestTheTunnelIsBoundToTheInterfaceAndInstallsNoRoutes(t *testing.T) {
	configuration := ikev2SwanctlConfiguration(*ikev2EAPProfile().IKEv2, *ikev2Secrets().IKEv2, nil)
	for _, want := range []string{
		fmt.Sprintf("if_id_in = %d\n", ikev2Link.ID), fmt.Sprintf("if_id_out = %d\n", ikev2Link.ID),
		"vips = 0.0.0.0\n", "remote_ts = 0.0.0.0/0\n",
	} {
		if !strings.Contains(configuration, want) {
			t.Errorf("swanctl.conf に %q が無い:\n%s", want, configuration)
		}
	}
	daemon := ikev2DaemonConfiguration()
	for _, want := range []string{
		"install_routes = no\n", "install_virtual_ip_on = " + ikev2Link.Name + "\n",
		"resolve {\n            load = no\n",
	} {
		if !strings.Contains(daemon, want) {
			t.Errorf("strongswan.conf に %q が無い:\n%s", want, daemon)
		}
	}
	if ikev2Link.ID == 0 {
		t.Fatal("if_id 0 はインターフェースに結び付かない")
	}
}

// 暗号スイートは L2TP/IPsec と同じ書き方で受け取る。末尾の ! は指定したものだけ、
// 無ければ既定の候補も後ろに提示する。
func TestIKEv2ProposalsMeanTheSameAsInL2TP(t *testing.T) {
	for _, test := range []struct{ ike, esp, want string }{
		{"", "", ""},
		{"aes256-sha256-modp2048", "", `proposals = "aes256-sha256-modp2048,default"`},
		{"aes256-sha256-modp2048!", "", `proposals = "aes256-sha256-modp2048"` + "\n"},
		{"", "aes128gcm16!", `esp_proposals = "aes128gcm16"` + "\n"},
		{"", "aes256-sha1,aes128-sha1", `esp_proposals = "aes256-sha1,aes128-sha1,default"`},
	} {
		profile := ikev2EAPProfile()
		profile.IKEv2.IKE, profile.IKEv2.ESP = test.ike, test.esp
		configuration := ikev2SwanctlConfiguration(*profile.IKEv2, *ikev2Secrets().IKEv2, nil)
		if test.want == "" {
			if strings.Contains(configuration, "proposals") {
				t.Errorf("指定していない暗号スイートが書かれた:\n%s", configuration)
			}
			continue
		}
		if !strings.Contains(configuration, test.want) {
			t.Errorf("ike=%q esp=%q: %q が無い:\n%s", test.ike, test.esp, test.want, configuration)
		}
	}
}

// agent へ渡す文書は、この backend に要るものだけを運ぶ。
func TestTheIKEv2DocumentCarriesWhatTheContainerNeeds(t *testing.T) {
	for _, test := range []struct {
		name              string
		profile           Profile
		authority         string
		publicAuthorities bool
	}{
		{"公的な認証局", ikev2EAPProfile(), "", true},
		{"指定した CA", ikev2EAPProfile(), newTestAuthority(t, "office CA").PEM, false},
		{"事前共有鍵", ikev2PSKProfile(), "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.profile.IKEv2.CACertificate = test.authority
			document, err := newAgentDocument(test.profile, ikev2Secrets(), testClock)
			if err != nil {
				t.Fatalf("newAgentDocument = %v", err)
			}
			var decoded agentDocument
			if err := json.Unmarshal([]byte(document), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.WireGuard != nil || decoded.L2TP != nil || decoded.OpenConnect != nil {
				t.Fatalf("ほかの方式の設定が混ざった: %s", document)
			}
			ikev2 := decoded.IKEv2
			if ikev2 == nil || ikev2.Server != "vpn.example.jp" || ikev2.Link != ikev2Link ||
				ikev2.PublicAuthorities != test.publicAuthorities {
				t.Fatalf("ikev2 = %+v", ikev2)
			}
			for _, name := range []string{"swanctl.conf", "strongswan.conf"} {
				if ikev2.Documents[name] == "" {
					t.Errorf("%s が無い", name)
				}
			}
			if _, present := ikev2.Documents["ca-1.pem"]; present != (test.authority != "") {
				t.Errorf("ca-1.pem の有無が違う: %v", present)
			}
		})
	}
}
