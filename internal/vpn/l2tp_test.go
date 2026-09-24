package vpn

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func l2tpProfile() Profile {
	return Profile{
		Name:    "tohoku",
		Backend: L2TPIPsec,
		L2TP: &L2TPSettings{
			Server:   "vpn.example.jp",
			Username: "vpn-user",
			ESP:      "aes256-sha256,aes128-sha1",
		},
	}
}

func l2tpSecrets() Secrets {
	return Secrets{L2TP: &L2TPSecrets{Password: `p"a\ss`, PreSharedKey: "shared-secret"}}
}

// 相手のアドレスは設定に書かず、コンテナの中で引いた値で置き換える。
func TestTheServerAddressIsLeftForTheContainerToResolve(t *testing.T) {
	documents := l2tpDocuments(*l2tpProfile().L2TP, *l2tpSecrets().L2TP)

	for _, name := range []string{"ipsec.conf", "xl2tpd.conf"} {
		if !strings.Contains(documents[name], serverAddressPlaceholder) {
			t.Errorf("%s は印を持たない: %s", name, documents[name])
		}
		if strings.Contains(documents[name], "vpn.example.jp") {
			t.Errorf("%s が名前のまま書かれている: %s", name, documents[name])
		}
	}
}

// トンネルは接続先への経路だけを作る。既定経路を奪わない。
func TestTheTunnelNeverTakesTheDefaultRoute(t *testing.T) {
	documents := l2tpDocuments(*l2tpProfile().L2TP, *l2tpSecrets().L2TP)

	if !strings.Contains(documents["ppp.options"], "nodefaultroute") {
		t.Fatalf("ppp.options = %s", documents["ppp.options"])
	}
	if strings.Contains(documents["ppp.options"], "defaultroute\n") &&
		!strings.Contains(documents["ppp.options"], "nodefaultroute\n") {
		t.Fatalf("ppp.options が既定経路を取る: %s", documents["ppp.options"])
	}
}

// 事前共有鍵は16進で書く。引用符や backslash を含んでも構文が壊れない。
func TestThePreSharedKeyIsWrittenAsHexadecimal(t *testing.T) {
	secrets := l2tpSecrets()
	documents := l2tpDocuments(*l2tpProfile().L2TP, *secrets.L2TP)

	want := ": PSK 0x" + hex.EncodeToString([]byte(secrets.L2TP.PreSharedKey)) + "\n"
	if documents["ipsec.secrets"] != want {
		t.Fatalf("ipsec.secrets = %q, want %q", documents["ipsec.secrets"], want)
	}
	if strings.Contains(documents["ipsec.secrets"], secrets.L2TP.PreSharedKey) {
		t.Fatalf("ipsec.secrets が素の鍵を含む: %q", documents["ipsec.secrets"])
	}
}

// 利用者名とパスワードは pppd の1語として書く。
func TestTheUserAndPasswordAreQuotedForPPP(t *testing.T) {
	documents := l2tpDocuments(*l2tpProfile().L2TP, *l2tpSecrets().L2TP)

	if !strings.Contains(documents["ppp.options"], `user "vpn-user"`) {
		t.Errorf("ppp.options = %s", documents["ppp.options"])
	}
	if !strings.Contains(documents["ppp.options"], `password "p\"a\\ss"`) {
		t.Errorf("ppp.options がパスワードを1語にしていない: %s", documents["ppp.options"])
	}
}

// 指定した暗号方式だけを書き、指定しなければ既定に任せる。
func TestOnlyTheGivenProposalsAreWritten(t *testing.T) {
	documents := l2tpDocuments(*l2tpProfile().L2TP, *l2tpSecrets().L2TP)
	if !strings.Contains(documents["ipsec.conf"], "    esp=aes256-sha256,aes128-sha1") {
		t.Errorf("ipsec.conf = %s", documents["ipsec.conf"])
	}
	if strings.Contains(documents["ipsec.conf"], "    ike=") {
		t.Errorf("指定していない ike が書かれた: %s", documents["ipsec.conf"])
	}
}

// agent へ渡す文書は、この backend に要る本文だけを運ぶ。
func TestTheL2TPDocumentCarriesEveryFileTheContainerNeeds(t *testing.T) {
	document, err := newAgentDocument(l2tpProfile(), l2tpSecrets(), testClock)
	if err != nil {
		t.Fatalf("newAgentDocument = %v", err)
	}

	var decoded agentDocument
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.WireGuard != nil {
		t.Fatalf("wireguard の設定が混ざった: %+v", decoded.WireGuard)
	}
	if decoded.L2TP == nil || decoded.L2TP.Server != "vpn.example.jp" {
		t.Fatalf("l2tp = %+v", decoded.L2TP)
	}
	for _, name := range []string{"ipsec.conf", "ipsec.secrets", "xl2tpd.conf", "ppp.options"} {
		if decoded.L2TP.Documents[name] == "" {
			t.Errorf("%s が無い", name)
		}
	}
}

// 表示するログには、パスワードも事前共有鍵も、その16進表記も残らない。
func TestShownLogsHideEveryL2TPSecret(t *testing.T) {
	secrets := l2tpSecrets()
	logs := strings.Join([]string{
		"xl2tpd: password " + secrets.L2TP.Password,
		"charon: psk " + hex.EncodeToString([]byte(secrets.L2TP.PreSharedKey)),
		"charon: PSK " + strings.ToUpper(hex.EncodeToString([]byte(secrets.L2TP.PreSharedKey))),
	}, "\n")

	shown := redact(logs, secrets)

	for _, forbidden := range []string{
		secrets.L2TP.Password,
		hex.EncodeToString([]byte(secrets.L2TP.PreSharedKey)),
		strings.ToUpper(hex.EncodeToString([]byte(secrets.L2TP.PreSharedKey))),
	} {
		if strings.Contains(shown, forbidden) {
			t.Fatalf("ログに %q が残った: %s", forbidden, shown)
		}
	}
}

// backend が要る秘密が無いまま繋ぎに行かない。
func TestL2TPSecretsAreRequiredBeforeConnecting(t *testing.T) {
	profile := l2tpProfile()
	for _, secrets := range []Secrets{
		{},
		{L2TP: &L2TPSecrets{Password: "only-password"}},
		{L2TP: &L2TPSecrets{PreSharedKey: "only-psk"}},
	} {
		if err := profile.ValidateSecrets(secrets); err == nil {
			t.Fatalf("ValidateSecrets accepted %+v", secrets)
		}
	}
	if err := profile.ValidateSecrets(l2tpSecrets()); err != nil {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}
