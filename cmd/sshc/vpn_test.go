package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/application"
	"sshc/internal/httpserver"
)

const testVPNKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="

func vpnOverviewFixture() string {
	// engine が返す形をそのまま使う。backend ごとの節も含む。
	return `{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
		`"target":"10.9.9.1:22","wireguard":{"server":"vpn.example.jp:51820",` +
		`"peerPublicKey":"bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=","address":"10.9.9.2/32"}},` +
		`"running":true,"relaySocket":"/home/u/.ssh/sshc/vpn/lab/relay.sock",` +
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
	payload, err := buildVPNProfilePayload(application.VPNProfile{
		Name: "lab", Backend: "wireguard", Target: "10.9.9.1:22",
		WireGuard: &application.WireGuardProfile{
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

// add は、既存のプロファイルを上書きしないよう、作成の口へ送る。
func TestAddingAProfileAsksTheEngineToCreateIt(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(vpnOverviewFixture()))
	})
	defer server.Close()
	engine, err := openEngineAPI(context.Background(), stateDir, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = engine.Close() }()
	var overview httpserver.VPNOverview

	if err := createVPNProfile(context.Background(), engine, []byte(`{"profile":{}}`), &overview); err != nil {
		t.Fatalf("createVPNProfile = %v", err)
	}

	if strings.Join(harness.methods, ",") != http.MethodPost ||
		strings.Join(harness.paths, ",") != "/api/v1/vpn/profiles" {
		t.Fatalf("requests = %v %v", harness.methods, harness.paths)
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
	payload, err := buildVPNProfilePayload(application.VPNProfile{
		Name: "tohoku", Backend: "l2tp_ipsec", Target: "10.9.9.1:22",
		L2TP: &application.L2TPProfile{Server: "vpn.example.jp", Username: "user"},
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

// ProxyCommand は、標準入出力を経路の中継へそのまま流す。
func TestTheProxyPipesStandardInputAndOutputThroughTheRoute(t *testing.T) {
	socket := filepath.Join(shortSocketDirectory(t), "relay.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	served := make(chan []byte, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			served <- nil
			return
		}
		defer func() { _ = connection.Close() }()
		received, _ := io.ReadAll(connection)
		_, _ = connection.Write([]byte("SSH-2.0-remote\r\n"))
		served <- received
	}()

	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
			`"target":"10.9.9.1:22"},"running":true,"relaySocket":` + jsonString(t, socket) + `,"connections":[]}]}`))
	})
	defer server.Close()
	stdin := writeTemporaryFile(t, "SSH-2.0-local\r\n")
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnProxy, Name: "lab", Target: "10.9.9.1:22"},
		commandEnvironment{stateDir: stateDir, client: server.Client(), stdin: stdin, stdout: &stdout, stderr: &stderr})

	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if sent := string(<-served); sent != "SSH-2.0-local\r\n" {
		t.Fatalf("中継へ届いた本文 = %q", sent)
	}
	if stdout.String() != "SSH-2.0-remote\r\n" {
		t.Fatalf("標準出力 = %q", stdout.String())
	}
}

// 設定に書いた相手と経路の接続先が違えば、通さない。
func TestTheProxyRefusesAConnectionTheRouteDoesNotReach(t *testing.T) {
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
			`"target":"10.9.9.1:22"},"running":true,"relaySocket":"/tmp/absent.sock","connections":[]}]}`))
	})
	defer server.Close()
	stdin := writeTemporaryFile(t, "")
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnProxy, Name: "lab", Target: "10.9.9.9:22"},
		commandEnvironment{stateDir: stateDir, client: server.Client(), stdin: stdin, stdout: &stdout, stderr: &stderr})

	if code == 0 {
		t.Fatal("経路が届かない相手へ通した")
	}
	if stdout.Len() != 0 {
		t.Fatalf("標準出力に何か書いた: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "10.9.9.1:22") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

// writeTemporaryFile は、標準入力として渡せるファイルを作る。
func writeTemporaryFile(t *testing.T, contents string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

// DNSの並びは、空白を挟んでいても一件ずつに分ける。書かなければ無しとして扱う。
func TestTheResolversAreReadOneAtATime(t *testing.T) {
	for _, test := range []struct {
		name, given string
		want        []string
	}{
		{"空なら無し", "", nil},
		{"空白だけなら無し", "  ,  ", nil},
		{"空白を挟んだ並び", " 10.9.9.53 , 10.9.9.54 ", []string{"10.9.9.53", "10.9.9.54"}},
		{"一件だけ", "10.9.9.53", []string{"10.9.9.53"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := splitVPNResolvers(test.given); strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("splitVPNResolvers(%q) = %v, want %v", test.given, got, test.want)
			}
		})
	}
}

// 立ち上がるまで待つあいだ、何を待っているかを言う。失敗したら次に読む場所も言う。
func TestBringingARouteUpSaysWhatItIsWaitingForAndWhereToLookWhenItFails(t *testing.T) {
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/problem+json")
		response.WriteHeader(http.StatusConflict)
		_, _ = response.Write([]byte(`{"code":"vpn_session_failed","message":"the tunnel did not come up"}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnUp, Name: "lab"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code == 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "VPNに接続しています") {
		t.Fatalf("待っているあいだの案内が無い: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "sshc vpn logs lab") {
		t.Fatalf("次に読む場所の案内が無い: %q", stderr.String())
	}
}

// 用意している途中の経路は、どこまで進んだかを一覧に出す。
func TestTheListSaysHowFarAStartingRouteHasGot(t *testing.T) {
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
			`"target":"10.9.9.1:22"},"running":true,"relaySocket":"","connections":[],"phase":"tunnel"}]}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnList}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "starting: waiting for the tunnel") {
		t.Fatalf("output = %q", stdout.String())
	}
}

// ログは engine から取り、そのまま見せる。
func TestVPNLogsArePrintedAsTheEngineReturnedThem(t *testing.T) {
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"lines":"IPsecを開始します。\n[REDACTED] を使いました\n"}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnLogsAction, Name: "lab"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if strings.Join(harness.paths, ",") != "/api/v1/vpn/profiles/lab/logs" {
		t.Fatalf("paths = %v", harness.paths)
	}
	if !strings.Contains(stdout.String(), "IPsecを開始します。") || !strings.Contains(stdout.String(), "[REDACTED]") {
		t.Fatalf("output = %q", stdout.String())
	}
}

// 改名は、新しい名前だけを engine へ渡す。
func TestVPNRenameSendsTheNewName(t *testing.T) {
	var sent map[string]string
	harness, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			_ = json.NewDecoder(request.Body).Decode(&sent)
		}
		_, _ = response.Write([]byte(vpnOverviewFixture()))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnRename, Name: "old", Rename: "new"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 || sent["name"] != "new" {
		t.Fatalf("code=%d sent=%v stderr=%q", code, sent, stderr.String())
	}
	if strings.Join(harness.paths, ",") != "/api/v1/vpn/profiles/old/rename" {
		t.Fatalf("paths = %v", harness.paths)
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
		{[]string{"vpn", "rename", "old", "new"}, true},
		{[]string{"vpn", "rename", "old"}, false},
		{[]string{"vpn", "logs", "lab"}, true},
		{[]string{"vpn", "logs", "lab", "--json"}, true},
		{[]string{"vpn", "logs"}, false},
		{[]string{"vpn", "proxy", "lab"}, true},
		{[]string{"vpn", "proxy", "lab", "10.9.9.1", "22"}, true},
		{[]string{"vpn", "proxy", "lab", "10.9.9.1"}, false},
		{[]string{"vpn", "proxy"}, false},
		{[]string{"vpn", "wat"}, false},
	} {
		called, err := parseInvocation(append([]string{"sshc"}, test.args...))
		accepted := err == nil && called.Kind == invocationVPN
		if accepted != test.accept {
			t.Errorf("%v accepted = %v (%v)", test.args, accepted, err)
		}
	}
}

// jsonString は、値を JSON の文字列として書く。Windows のパスの \ をそのまま
// 埋め込むと、JSON のエスケープとして壊れる。
func jsonString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
