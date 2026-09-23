package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const testVPNKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="

func vpnOverviewFixture() string {
	return `{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
		`"target":"10.9.9.1:22"},"running":true,"relaySocket":"/home/u/.ssh/sshc/vpn/lab/relay.sock",` +
		`"connections":["lab"]}]}`
}

// 一覧は、プロファイルと状態と、それを通る接続を見せる。
func TestVPNListShowsEachProfileWithItsSessionAndConnections(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(vpnOverviewFixture()))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnList}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Join(harness.paths, ",") != "/api/v1/vpn" {
		t.Fatalf("paths = %v", harness.paths)
	}
	for _, want := range []string{"lab", "wireguard", "10.9.9.1:22", "up", "connections: lab"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output omitted %q: %s", want, stdout.String())
		}
	}
}

// 経路を作れない機械では、理由をそのまま見せる。
func TestVPNListSaysWhyTheMachineCannotOpenRoutes(t *testing.T) {
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available":false,"detail":"docker is not available","profiles":[]}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnList}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "docker is not available") {
		t.Fatalf("output = %q", stdout.String())
	}
}

// 紐付けは、alias とプロファイル名だけを engine へ渡す。
func TestVPNBindSendsTheAliasAndProfile(t *testing.T) {
	var sent map[string]string
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			if err := json.NewDecoder(request.Body).Decode(&sent); err != nil {
				t.Error(err)
			}
			_, _ = response.Write([]byte(vpnOverviewFixture()))
			return
		}
		_, _ = response.Write([]byte(vpnOverviewFixture()))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnBind, Alias: "lab", Name: "tohoku"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if sent["alias"] != "lab" || sent["profile"] != "tohoku" {
		t.Fatalf("sent = %v", sent)
	}
	if strings.Join(harness.paths, ",") != "/api/v1/vpn/bindings" {
		t.Fatalf("paths = %v", harness.paths)
	}
}

// 紐付けを外すときは、プロファイル名を空で送る。
func TestVPNUnbindSendsAnEmptyProfile(t *testing.T) {
	var sent map[string]string
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPut {
			_ = json.NewDecoder(request.Body).Decode(&sent)
		}
		_, _ = response.Write([]byte(vpnOverviewFixture()))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnUnbind, Alias: "lab"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || sent["alias"] != "lab" || sent["profile"] != "" {
		t.Fatalf("code=%d sent=%v stderr=%q", code, sent, stderr.String())
	}
}

// 保存要求の本文は、設定と秘密鍵をひとつのJSONとして運ぶ。
func TestTheSavedProfilePayloadCarriesTheKeyExactlyOnce(t *testing.T) {
	payload, err := buildVPNProfilePayload(vpnRequestProfile{
		Name: "lab", Backend: "wireguard", Target: "10.9.9.1:22",
		WireGuard: &vpnRequestWireGuard{
			Server: "vpn.example.jp:51820", PeerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=",
			Address: "10.9.9.2/32",
		},
	}, []vpnSecretField{{name: "wireguardPrivateKey", value: []byte(testVPNKey)}})
	if err != nil {
		t.Fatalf("buildVPNProfilePayload = %v", err)
	}

	var decoded struct {
		Profile struct {
			Name      string `json:"name"`
			Target    string `json:"target"`
			WireGuard struct {
				Server string `json:"server"`
			} `json:"wireguard"`
		} `json:"profile"`
		Secrets struct {
			WireGuardPrivateKey string `json:"wireguardPrivateKey"`
		} `json:"secrets"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload = %s: %v", payload, err)
	}
	if decoded.Profile.Name != "lab" || decoded.Profile.Target != "10.9.9.1:22" ||
		decoded.Profile.WireGuard.Server != "vpn.example.jp:51820" {
		t.Fatalf("profile = %+v", decoded.Profile)
	}
	if decoded.Secrets.WireGuardPrivateKey != testVPNKey {
		t.Fatalf("secret = %q", decoded.Secrets.WireGuardPrivateKey)
	}
	if strings.Count(string(payload), testVPNKey) != 1 {
		t.Fatalf("鍵が本文に複数回現れた: %s", payload)
	}
}

// 鍵の形が違うものは、engine へ送る前に断る。
func TestAMalformedKeyIsNotAcceptedAsAWireGuardSecret(t *testing.T) {
	for _, key := range []string{"", `has"quote`, strings.Repeat("a", maxVPNKeyBytes+1)} {
		if base64KeyBytes([]byte(key)) && len(key) <= maxVPNKeyBytes {
			t.Fatalf("base64KeyBytes accepted %q", key)
		}
	}
}

// 引用符や backslash を含む秘密も、壊れないJSONとして運ぶ。
func TestSecretsWithQuotesSurviveTheRequestBody(t *testing.T) {
	password := []byte(`p"a\ss`)
	payload, err := buildVPNProfilePayload(vpnRequestProfile{
		Name: "tohoku", Backend: "l2tp_ipsec", Target: "10.9.9.1:22",
		L2TP: &vpnRequestL2TP{Server: "vpn.example.jp", Username: "user"},
	}, []vpnSecretField{
		{name: "l2tpPassword", value: password},
		{name: "ipsecPsk", value: []byte("shared")},
	})
	if err != nil {
		t.Fatalf("buildVPNProfilePayload = %v", err)
	}

	var decoded struct {
		Profile struct {
			L2TP struct {
				Server   string `json:"server"`
				Username string `json:"username"`
			} `json:"l2tp"`
		} `json:"profile"`
		Secrets struct {
			L2TPPassword string `json:"l2tpPassword"`
			IPsecPSK     string `json:"ipsecPsk"`
		} `json:"secrets"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload = %s: %v", payload, err)
	}
	if decoded.Secrets.L2TPPassword != string(password) || decoded.Secrets.IPsecPSK != "shared" {
		t.Fatalf("secrets = %+v", decoded.Secrets)
	}
	if decoded.Profile.L2TP.Server != "vpn.example.jp" || decoded.Profile.L2TP.Username != "user" {
		t.Fatalf("l2tp = %+v", decoded.Profile.L2TP)
	}
}

// 受け取り方を間違えた呼び出しは、engine へ届く前に断る。
func TestVPNInvocationsAreAcceptedOnlyInTheirDocumentedShapes(t *testing.T) {
	for _, test := range []struct {
		args   []string
		accept bool
	}{
		{[]string{"vpn"}, true},
		{[]string{"vpn", "--json"}, true},
		{[]string{"vpn", "add", "lab"}, true},
		{[]string{"vpn", "add"}, false},
		{[]string{"vpn", "add", "lab", "extra"}, false},
		{[]string{"vpn", "remove", "lab"}, true},
		{[]string{"vpn", "remove", "lab", "--yes"}, true},
		{[]string{"vpn", "remove", "lab", "--force"}, false},
		{[]string{"vpn", "up", "lab", "--json"}, true},
		{[]string{"vpn", "down", "lab"}, true},
		{[]string{"vpn", "down"}, false},
		{[]string{"vpn", "bind", "host", "lab"}, true},
		{[]string{"vpn", "bind", "host"}, false},
		{[]string{"vpn", "unbind", "host"}, true},
		{[]string{"vpn", "unbind"}, false},
		{[]string{"vpn", "wat"}, false},
	} {
		called, err := parseInvocation(append([]string{"sshc"}, test.args...))
		accepted := err == nil && called.Kind == invocationVPN
		if accepted != test.accept {
			t.Errorf("%v accepted = %v (%v)", test.args, accepted, err)
		}
	}
}
