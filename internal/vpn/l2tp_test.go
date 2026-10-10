package vpn

import (
	"encoding/hex"
	"encoding/json"
	"slices"
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

func TestExplicitProposalsArePreserved(t *testing.T) {
	settings := *l2tpProfile().L2TP
	settings.IKE = "aes256-sha1-modp2048!"
	documents := l2tpDocuments(settings, *l2tpSecrets().L2TP)
	for _, want := range []string{"    ike=aes256-sha1-modp2048!\n", "    esp=aes256-sha256,aes128-sha1\n"} {
		if !strings.Contains(documents["ipsec.conf"], want) {
			t.Errorf("ipsec.conf に %q が無い:\n%s", want, documents["ipsec.conf"])
		}
	}
}

// IKEv1 では proposal ごとに暗号・MAC・DH群の先頭の1つしか送られないので、既定の IKE は
// 組み合わせを1つずつ並べる。末尾に ! を付けず、strongSwan の既定の候補も後ろに残す。
func TestAnUnconfiguredIKEOffersEachSuiteAsItsOwnProposal(t *testing.T) {
	settings := *l2tpProfile().L2TP
	settings.IKE = ""
	documents := l2tpDocuments(settings, *l2tpSecrets().L2TP)
	if !strings.Contains(documents["ipsec.conf"], "    ike="+defaultL2TPIKE+"\n") {
		t.Fatalf("既定の IKE が書かれていない: %s", documents["ipsec.conf"])
	}
	proposals := strings.Split(defaultL2TPIKE, ",")
	for _, want := range []string{"aes256-sha1-modp2048", "aes128-sha1-modp1024", "aes128-sha256-modp3072"} {
		if !slices.Contains(proposals, want) {
			t.Errorf("既定の IKE に %s が無い: %s", want, defaultL2TPIKE)
		}
	}
	for _, proposal := range proposals {
		if strings.Count(proposal, "-") != 2 || strings.HasSuffix(proposal, "!") {
			t.Errorf("1つの組み合わせでない候補がある: %s", proposal)
		}
	}
}

func TestAnUnconfiguredESPIncludesSeparateSHA1ProposalsForL2TPServers(t *testing.T) {
	settings := *l2tpProfile().L2TP
	settings.ESP = ""
	documents := l2tpDocuments(settings, *l2tpSecrets().L2TP)
	if !strings.Contains(documents["ipsec.conf"], "    esp=aes256-sha256,aes128-sha256,aes256-sha1,aes128-sha1\n") {
		t.Fatalf("default ESP omits IKEv1-compatible proposals: %s", documents["ipsec.conf"])
	}
}

// charon のログの既定の書き先は syslog で、コンテナの中には受け取る相手がいない。
// agent が失敗したときに見せる ipsec.log へ、書くたびに書き出させる。
func TestCharonWritesItsLogToTheFileTheAgentShows(t *testing.T) {
	documents := l2tpDocuments(*l2tpProfile().L2TP, *l2tpSecrets().L2TP)
	daemon := documents["strongswan.conf"]
	for _, want := range []string{
		"include /etc/strongswan.conf\n",
		"path = " + agentRuntimeDirectory + "/ipsec.log\n",
		"flush_line = yes\n",
	} {
		if !strings.Contains(daemon, want) {
			t.Errorf("strongswan.conf に %q が無い:\n%s", want, daemon)
		}
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
	for _, name := range []string{"strongswan.conf", "ipsec.conf", "ipsec.secrets", "xl2tpd.conf", "ppp.options"} {
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

	shown := redactLogs(logs, secrets)

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

// パスワードが事前共有鍵の頭と同じでも、事前共有鍵の残りをログに出さない。
func TestShownLogsHideAPreSharedKeyThatStartsWithThePassword(t *testing.T) {
	secrets := Secrets{L2TP: &L2TPSecrets{Password: "hunter2", PreSharedKey: "hunter2-psk-value"}}
	logs := "charon: psk hunter2-psk-value\ncharon: hex " + hex.EncodeToString([]byte("hunter2-psk-value"))

	shown := redactLogs(logs, secrets)

	if shown != "charon: psk [REDACTED]\ncharon: hex [REDACTED]" {
		t.Fatalf("ログ: %q", shown)
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
