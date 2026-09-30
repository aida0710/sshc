package vpn

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// テストの鍵である。どれも 44 字の base64 で、値そのものに意味は無い。
const (
	testSecondPublicKey = "cCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCcCA="
	testPresharedKey    = "dDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDdDA="
)

// testWireGuardConfig は、Peer を2つ持ち、片方に PresharedKey を付けた設定ファイルである。
var testWireGuardConfig = strings.Join([]string{
	"[Interface]",
	"PrivateKey = " + testPrivateKey,
	"Address = 10.64.1.2/32, fc00::2/128",
	"DNS = 10.64.0.1",
	"MTU = 1380",
	"Table = off",
	"ListenPort = 51821",
	"",
	"[Peer]",
	"PublicKey = " + testPublicKey,
	"PresharedKey = " + testPresharedKey,
	"Endpoint = vpn.example.jp:51820",
	"AllowedIPs = 10.64.0.0/16",
	"AllowedIPs = 10.65.0.0/16",
	"",
	"[Peer]",
	"PublicKey = " + testSecondPublicKey,
	"AllowedIPs = 10.66.0.0/16",
	"",
}, "\n")

func wireGuardConfigProfile() Profile {
	return Profile{
		Name: "provider", Backend: WireGuard, DNS: []string{"10.64.0.1"},
		WireGuard: &WireGuardSettings{Servers: []string{"vpn.example.jp"}},
	}
}

func wireGuardConfigSecrets() Secrets {
	return Secrets{WireGuard: &WireGuardSecrets{Config: testWireGuardConfig}}
}

// コンテナへは、wg setconf が読む形に直して渡す。wg-quick だけが読む項目と注釈は書かない。
// AllowedIPs は利用者が書いたものを使う。
func TestWireGuardIsGivenOnlyWhatWgSetconfReads(t *testing.T) {
	document := decodeAgentDocument(t, wireGuardConfigProfile(), wireGuardConfigSecrets())

	configuration := document.WireGuard.Configuration
	for _, wanted := range []string{
		"PrivateKey = " + testPrivateKey, "ListenPort = 51821",
		"PublicKey = " + testPublicKey, "PresharedKey = " + testPresharedKey,
		"Endpoint = vpn.example.jp:51820", "AllowedIPs = 10.64.0.0/16, 10.65.0.0/16",
		"PublicKey = " + testSecondPublicKey, "AllowedIPs = 10.66.0.0/16",
	} {
		if !strings.Contains(configuration, wanted+"\n") {
			t.Errorf("configuration に %q が無い:\n%s", wanted, configuration)
		}
	}
	for _, unwanted := range []string{"Address", "DNS", "MTU", "Table"} {
		if strings.Contains(configuration, unwanted) {
			t.Errorf("configuration に %q がある:\n%s", unwanted, configuration)
		}
	}
	// Address と MTU は、wg setconf の代わりに agent が ip で設定する。IPv6 は使わない。
	if strings.Join(document.WireGuard.Addresses, ",") != "10.64.1.2/32" || document.WireGuard.MTU != 1380 {
		t.Fatalf("addresses = %v, mtu = %d", document.WireGuard.Addresses, document.WireGuard.MTU)
	}
	if strings.Join(document.DNS, ",") != "10.64.0.1" {
		t.Fatalf("dns = %v", document.DNS)
	}
}

// Endpoint のある Peer に keepalive が無ければ、sshc の間隔を使う。ハンドシェイクが起きない
// と、sshc はトンネルが張れたことも、生きていることも確かめられない。Endpoint の無い Peer
// には足さない。
func TestAnEndpointWithoutKeepaliveGetsTheSshcInterval(t *testing.T) {
	configuration := decodeAgentDocument(t, wireGuardConfigProfile(), wireGuardConfigSecrets()).WireGuard.Configuration

	peers := strings.Split(configuration, "[Peer]")
	if len(peers) != 3 || !strings.Contains(peers[1], "PersistentKeepalive = 25\n") ||
		strings.Contains(peers[2], "PersistentKeepalive") {
		t.Fatalf("configuration =\n%s", configuration)
	}

	secrets := Secrets{WireGuard: &WireGuardSecrets{Config: strings.Replace(testWireGuardConfig,
		"AllowedIPs = 10.65.0.0/16\n", "AllowedIPs = 10.65.0.0/16\nPersistentKeepalive = 10\n", 1)}}
	configured := decodeAgentDocument(t, wireGuardConfigProfile(), secrets).WireGuard.Configuration
	if !strings.Contains(configured, "PersistentKeepalive = 10\n") {
		t.Fatalf("書いた keepalive を使っていない:\n%s", configured)
	}
}

// 設定ファイルは、利用者が書いたとおりの本文のまま Vault に置く。書き直さない。
func TestTheConfigIsKeptAsWritten(t *testing.T) {
	own := wireGuardConfigProfile().OwnSecrets(Secrets{
		WireGuard: &WireGuardSecrets{Config: testWireGuardConfig},
		L2TP:      &L2TPSecrets{Password: "not used"},
	})

	if own.WireGuard == nil || own.WireGuard.Config != testWireGuardConfig || own.L2TP != nil {
		t.Fatalf("OwnSecrets = %+v", own)
	}
}

// 設定ファイルの行の誤りは、シークレットの項目として、行と項目の名前を添えて断る。
func TestAConfigRefusalNamesTheSecretFieldAndTheLine(t *testing.T) {
	secrets := Secrets{WireGuard: &WireGuardSecrets{
		Config: strings.Replace(testWireGuardConfig, "Table = off", "PostUp = iptables -A FORWARD", 1),
	}}

	err := wireGuardConfigProfile().ValidateSecrets(secrets)

	var refused *ConfigLineError
	if !errors.As(err, &refused) || refused.Field != "secrets.wireguardConfig" || refused.Reason != ReasonRunsCommand ||
		refused.Line != 6 || refused.Directive != "PostUp" || !errors.Is(err, ErrSecrets) {
		t.Fatalf("ValidateSecrets = %v", err)
	}
}

// ログからは、秘密鍵も PresharedKey も伏せる。
func TestShownLogsHideTheWireGuardKeys(t *testing.T) {
	logs := "key " + testPrivateKey + " psk " + testPresharedKey

	shown := redactLogs(logs, wireGuardConfigSecrets())

	if strings.Contains(shown, testPrivateKey) || strings.Contains(shown, testPresharedKey) {
		t.Fatalf("redact = %q", shown)
	}
}

// v0.40.0 までの項目の形のプロファイルは、同じ接続先へ届く設定ファイルになる。
func TestFieldsBecomeAConfigThatReachesTheSameTargets(t *testing.T) {
	fields := WireGuardFields{
		Server: "vpn.example.jp:51820", PeerPublicKey: testPublicKey, Address: "10.9.9.2/32", DNS: []string{"10.9.9.53"},
	}
	if err := fields.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	profile := Profile{
		Name: "lab", Backend: WireGuard, DNS: fields.DNS, WireGuard: &WireGuardSettings{Servers: fields.Servers()},
	}
	secrets := Secrets{WireGuard: &WireGuardSecrets{Config: fields.Config(testPrivateKey)}}
	if err := profile.ValidateSecrets(secrets); err != nil {
		t.Fatalf("ValidateSecrets = %v\n%s", err, secrets.WireGuard.Config)
	}
	configuration := decodeAgentDocument(t, profile, secrets).WireGuard.Configuration
	for _, wanted := range []string{"PrivateKey = " + testPrivateKey, "Endpoint = vpn.example.jp:51820",
		"AllowedIPs = 0.0.0.0/0", "PersistentKeepalive = 25"} {
		if !strings.Contains(configuration, wanted+"\n") {
			t.Fatalf("configuration に %q が無い:\n%s", wanted, configuration)
		}
	}
	fields.PeerPublicKey = "short"
	if err := fields.Validate(); err == nil {
		t.Fatal("組み立てられない項目を通した")
	}
}

// 接続先が、どの Peer の AllowedIPs にも無ければ、connect は理由を返して繋ぎに行かない。
func TestATargetOutsideTheAllowedIPsIsRefusedByTheContainer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the container backend requires a POSIX shell")
	}
	directory := t.TempDir()
	stageBackendScript(t, directory, WireGuard)
	script := `runtime=$1
backend_directory=$runtime
. "$runtime/backend.sh"
wg() { printf 'peer1\t10.64.0.0/16 192.168.1.0/24\npeer2\t(none)\n'; }
fail() { printf 'reason=%s\n' "$1"; exit 1; }
backend_allow "$2" && echo allowed
`
	for target, want := range map[string]string{
		"10.64.3.4": "allowed", "192.168.1.200": "allowed", "10.65.0.1": "reason=target_not_allowed",
	} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		output, _ := exec.CommandContext(ctx, "sh", "-c", script, "wireguard-test", directory, target).CombinedOutput()
		cancel()
		if strings.TrimSpace(string(output)) != want {
			t.Errorf("%s: output = %q, want %q", target, output, want)
		}
	}
}

// WireGuard と OpenVPN のコンテナには、使う権限だけを渡す。中継のソケットは engine が
// ホスト側で作るので、所有者を変える権限（CHOWN）は渡さない。
func TestWireGuardAndOpenVPNContainersGetOnlyTheCapabilitiesTheyUse(t *testing.T) {
	want := "--cap-drop ALL --cap-add NET_ADMIN --cap-add NET_RAW --cap-add DAC_OVERRIDE"
	for _, name := range []BackendName{WireGuard, OpenVPN} {
		if got := strings.Join(backends[name].capabilities(), " "); got != want {
			t.Errorf("%s の権限 = %q, want %q", name, got, want)
		}
	}
}
