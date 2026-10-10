package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/application"
	"sshc/internal/httpserver"
	"sshc/internal/vpn"
)

const testVPNKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="

func vpnOverviewFixture() string {
	// engine が返す形をそのまま使う。backend ごとの節も含む。
	return `{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard",` +
		`"wireguard":{"servers":["vpn.example.jp"]}},` +
		`"running":true,"relaySocket":"/home/u/.ssh/sshc/vpn/lab/engine.sock",` +
		`"connections":["lab"]}]}`
}

// 一覧は、プロファイルと状態と、それを通る接続を見せる。
func TestVPNListShowsEachProfileWithItsRouteAndConnections(t *testing.T) {
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
	for _, want := range []string{"lab", "wireguard", "up", "connections: lab"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output omitted %q: %s", want, stdout.String())
		}
	}
}

// 経路を作れないマシンでは、理由を英語の文で見せ、docker の生の文を添える。
func TestVPNListSaysWhyTheMachineCannotOpenRoutes(t *testing.T) {
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"available":false,"unavailable":"vpn_docker_missing",` +
			`"detail":"docker is not installed: executable file not found","profiles":[]}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnList}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{"Docker was not found.", "Detail: docker is not installed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output omitted %q: %q", want, stdout.String())
		}
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

// 保存要求の本文は、設定とシークレットの設定ファイルをひとつのJSONとして運ぶ。
func TestTheSavedProfilePayloadCarriesTheKeyExactlyOnce(t *testing.T) {
	config := "[Interface]\nPrivateKey = " + testVPNKey + "\n"
	payload, err := buildVPNProfilePayload(application.VPNProfile{
		Name: "lab", Backend: "wireguard",
		WireGuard: &application.WireGuardProfile{Servers: []string{"vpn.example.jp"}},
	}, []vpnSecretField{{name: vpn.SecretKeyWireGuardConfig, value: []byte(config)}})
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
	if decoded.Profile.Name != "lab" || decoded.Profile.WireGuard.Servers[0] != "vpn.example.jp" {
		t.Fatalf("profile = %+v", decoded.Profile)
	}
	if decoded.Secrets.WireGuardConfig != config {
		t.Fatalf("secret = %q", decoded.Secrets.WireGuardConfig)
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

// 引用符や backslash を含む秘密も、壊れないJSONとして運ぶ。
func TestSecretsWithQuotesSurviveTheRequestBody(t *testing.T) {
	password := []byte(`p"a\ss`)
	payload, err := buildVPNProfilePayload(application.VPNProfile{
		Name: "tohoku", Backend: "l2tp_ipsec",
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

// relayConnected は、engine の中継が接続先へ繋げたときに1行目へ答える JSON である。
const relayConnected = "{}"

// relayStub は、engine の中継の代わりに、1行目の接続先を受け取って reply を答える。
// 受け取った接続先は requested へ渡す。答えが relayConnected なら、そのあとの接続を
// carry に任せ、carry が戻ったら閉じる。ほかの答えなら carry は呼ばない。
func relayStub(t *testing.T, reply string, carry func(net.Conn)) (socket string, requested <-chan string) {
	t.Helper()
	socket = filepath.Join(shortSocketDirectory(t), "engine.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	addresses := make(chan string, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			addresses <- ""
			return
		}
		defer func() { _ = connection.Close() }()
		addresses <- readRequestedAddress(connection)
		_, _ = connection.Write([]byte(reply + "\n"))
		if reply == relayConnected {
			carry(connection)
		}
	}()
	return socket, addresses
}

// readRequestedAddress は、繋ぐ側が1行目に送った接続先を読む。
func readRequestedAddress(connection net.Conn) string {
	line := []byte{}
	one := make([]byte, 1)
	for {
		if _, err := io.ReadFull(connection, one); err != nil || one[0] == '\n' {
			return string(line)
		}
		line = append(line, one[0])
	}
}

// relayOverview は、経路が起動していて、その中継が socket にある一覧である。
func relayOverview(t *testing.T, socket string) string {
	return `{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard"},` +
		`"running":true,"relaySocket":` + jsonString(t, socket) + `,"connections":[]}]}`
}

// relayEngineEnvironment は、経路の中継が socket にあると答える engine を立て、その
// engine へ繋ぐ環境を返す。標準入出力は呼び出し側が埋める。
func relayEngineEnvironment(t *testing.T, socket string) commandEnvironment {
	t.Helper()
	_, server, stateDir := newSyncCommandHarness(t, func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(relayOverview(t, socket)))
	})
	t.Cleanup(server.Close)
	return commandEnvironment{stateDir: stateDir, client: server.Client()}
}

// proxyReturnTimeout は、proxy が戻らないのを固まったと見なすまでの待ち。遅い CI の
// マシンでも、中継が閉じてから戻るまでには十分な長さにする。
const proxyReturnTimeout = 5 * time.Second

// runProxyWithinTimeout は、environment で sshc vpn proxy を走らせ、終了コードを返す。
// proxyReturnTimeout までに戻らなければ、stuck を理由にテストを落とす。go test の
// 上限まで待つと、パッケージごと打ち切られて、どこで止まったかが分かりにくい。
func runProxyWithinTimeout(t *testing.T, environment commandEnvironment, stuck string) int {
	t.Helper()
	code := make(chan int, 1)
	go func() {
		code <- runVPN(context.Background(), vpnInvocation{Action: vpnProxy, Name: "lab", Target: "10.9.9.1:22"},
			environment)
	}()
	select {
	case got := <-code:
		return got
	case <-time.After(proxyReturnTimeout):
		t.Fatal(stuck)
		return 0
	}
}

// ProxyCommand は、接続先を engine の中継へ伝えてから、標準入力を中継へ、中継から
// 届いたものを標準出力へ運ぶ。
//
// 中継は標準入力に置いたバイト数だけ読み、答えを返して閉じる。標準入力の終わりが中継へ
// 届くことには頼らない。それは TestTheProxyTellsTheRelayWhenStandardInputEnds が確かめる。
func TestTheProxyPipesStandardInputAndOutputThroughTheRoute(t *testing.T) {
	const sent, greeting = "SSH-2.0-local\r\n", "SSH-2.0-remote\r\n"
	served := make(chan string, 1)
	socket, requested := relayStub(t, relayConnected, func(connection net.Conn) {
		received := make([]byte, len(sent))
		read, _ := io.ReadFull(connection, received)
		served <- string(received[:read])
		_, _ = connection.Write([]byte(greeting))
	})
	environment := relayEngineEnvironment(t, socket)
	environment.stdin = writeTemporaryFile(t, sent)
	var stdout, stderr strings.Builder
	environment.stdout, environment.stderr = &stdout, &stderr

	code := runProxyWithinTimeout(t, environment, "中継が閉じたのに proxy が戻らない")

	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if address := <-requested; address != "10.9.9.1:22" {
		t.Fatalf("中継へ伝えた接続先 = %q", address)
	}
	if body := <-served; body != sent {
		t.Fatalf("中継へ届いた本文 = %q", body)
	}
	if stdout.String() != greeting {
		t.Fatalf("標準出力 = %q", stdout.String())
	}
}

// 標準入力が終わったら、送る側だけを閉じて中継へ伝え、中継から届くものは受け取り
// 続ける。伝えないと、入力の終わりを待って答える接続先とのあいだで止まったままになる。
//
// Windows では、相手が CloseWrite するのと同時に読み始めた Read が起こされず、止まった
// ままになることがある。Go の net の TestCloseWrite も、Windows と macOS/arm64 では読むのと
// 閉じるのが重ならないよう待っている（go.dev/issue/49352）。そのときは proxyReturnTimeout で
// 落ちる。
func TestTheProxyTellsTheRelayWhenStandardInputEnds(t *testing.T) {
	const farewell = "bye\r\n"
	endOfInput := make(chan error, 1)
	socket, _ := relayStub(t, relayConnected, func(connection net.Conn) {
		// 標準入力は空なので、最初の Read が入力の終わりを読む。
		_, err := connection.Read(make([]byte, 1))
		endOfInput <- err
		_, _ = connection.Write([]byte(farewell))
	})
	environment := relayEngineEnvironment(t, socket)
	environment.stdin = writeTemporaryFile(t, "")
	var stdout, stderr strings.Builder
	environment.stdout, environment.stderr = &stdout, &stderr

	code := runProxyWithinTimeout(t, environment, "標準入力の終わりが中継へ届かず、proxy と中継が互いを待ち続けた")

	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if err := <-endOfInput; !errors.Is(err, io.EOF) {
		t.Fatalf("中継の Read = %v, want EOF", err)
	}
	// 接続全体を閉じていれば、中継が終わりを読んだあとに返したものは届かない。
	if stdout.String() != farewell {
		t.Fatalf("標準出力 = %q", stdout.String())
	}
}

// 接続先の切断、sshc vpn down、engine の終了で中継が先に閉じたら、proxy は標準入力が
// 開いたままでも戻る。ssh は ProxyCommand の標準出力が閉じるまで切断に気付かない。
func TestTheProxyReturnsWhenTheRelayClosesFirstWhileStandardInputStaysOpen(t *testing.T) {
	const greeting = "SSH-2.0-remote\r\n"
	closings := map[string]func(net.Conn){
		"中継が接続を閉じる": func(connection net.Conn) { _ = connection.Close() },
		"中継が送る側だけを閉じる": func(connection net.Conn) {
			_ = connection.(*net.UnixConn).CloseWrite()
			_, _ = io.Copy(io.Discard, connection)
		},
	}
	for name, closeRelay := range closings {
		t.Run(name, func(t *testing.T) {
			socket, _ := relayStub(t, relayConnected, func(connection net.Conn) {
				_, _ = connection.Write([]byte(greeting))
				closeRelay(connection)
			})
			environment := relayEngineEnvironment(t, socket)
			stdin, stdinWriter, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stdinWriter.Close(); _ = stdin.Close() })
			environment.stdin = stdin
			var stdout, stderr strings.Builder
			environment.stdout, environment.stderr = &stdout, &stderr

			code := runProxyWithinTimeout(t, environment, "中継が閉じたのに proxy が標準入力を待ち続けた")

			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			if stdout.String() != greeting {
				t.Fatalf("標準出力 = %q", stdout.String())
			}
		})
	}
}

// 接続先へ繋げなかったときは、engine が答えた理由を英語の文で出し、標準出力には
// 何も書かない。
func TestTheProxySaysWhyTheRouteCouldNotReachTheDestination(t *testing.T) {
	socket, _ := relayStub(t, `{"code":"vpn_target_failed","reason":"target_unresolved"}`, nil)
	environment := relayEngineEnvironment(t, socket)
	environment.stdin = writeTemporaryFile(t, "")
	var stdout, stderr strings.Builder
	environment.stdout, environment.stderr = &stdout, &stderr

	code := runVPN(context.Background(), vpnInvocation{Action: vpnProxy, Name: "lab", Target: "db.internal:22"},
		environment)

	if code != 1 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("標準出力に何か書いた: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "could not resolve the destination") {
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
		_, _ = response.Write([]byte(`{"code":"vpn_route_failed","message":"the tunnel did not come up"}`))
	})
	defer server.Close()
	var stdout, stderr strings.Builder

	code := runVPN(context.Background(), vpnInvocation{Action: vpnUp, Name: "lab"}, commandEnvironment{
		stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr,
	})

	if code == 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), `Connecting to the VPN "lab".`) {
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
		_, _ = response.Write([]byte(`{"available":true,"profiles":[{"profile":{"name":"lab","backend":"wireguard"},` +
			`"running":true,"relaySocket":"","connections":[],"phase":"tunnel"}]}`))
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

// 経路を用意しているどの段階も、一覧の状態の欄では段階の語のままでなく英語の言葉で言う。
func TestEveryStartPhaseHasAStateWordInTheList(t *testing.T) {
	for _, phase := range []vpn.StartPhase{vpn.PhaseImage, vpn.PhaseContainer, vpn.PhaseTunnel, vpn.PhaseApproval} {
		if word := vpnPhaseWord(phase); word == string(phase) {
			t.Errorf("%s に状態の欄の語が無い", phase)
		}
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

// 空白と日本語を含む名前も、そのまま1つの名前として engine へ届く。
func TestVPNRenameCarriesNamesWithSpacesIntact(t *testing.T) {
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

	code := runVPN(context.Background(), vpnInvocation{Action: vpnRename, Name: "研究室 VPN", Rename: "研究室, 別館"},
		commandEnvironment{stateDir: stateDir, client: server.Client(), stdout: &stdout, stderr: &stderr})

	if code != 0 || sent["name"] != "研究室, 別館" {
		t.Fatalf("code=%d sent=%v stderr=%q", code, sent, stderr.String())
	}
	if strings.Join(harness.paths, ",") != "/api/v1/vpn/profiles/研究室 VPN/rename" {
		t.Fatalf("paths = %v", harness.paths)
	}
}

// 一覧の列は、全角の字を2桁と数えて揃える。
func TestTheVPNListAlignsJapaneseNamesByTheirWidthOnTheTerminal(t *testing.T) {
	var output strings.Builder

	writeAlignedRows(&output, [][2]string{{"研究室 VPN", "wireguard  up"}, {"lab", "openvpn  stopped"}})

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 2 || lines[0] != "研究室 VPN  wireguard  up" || lines[1] != "lab         openvpn  stopped" {
		t.Fatalf("output:\n%s", output.String())
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
		{[]string{"vpn", "edit", "lab"}, true},
		{[]string{"vpn", "edit"}, false},
		{[]string{"vpn", "edit", "lab", "--json"}, false},
		{[]string{"vpn", "rename", "old", "new"}, true},
		{[]string{"vpn", "rename", "old", "new", "--json"}, true},
		{[]string{"vpn", "rename", "old", "new", "--yes"}, false},
		{[]string{"vpn", "rename", "old"}, false},
		{[]string{"vpn", "logs", "lab"}, true},
		{[]string{"vpn", "logs", "lab", "--json"}, true},
		{[]string{"vpn", "logs"}, false},
		{[]string{"vpn", "proxy", "lab"}, false},
		{[]string{"vpn", "proxy", "lab", "10.9.9.1", "22"}, true},
		{[]string{"vpn", "proxy", "研究室 VPN", "10.9.9.1", "22"}, true},
		{[]string{"vpn", "up", "研究室 VPN"}, true},
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
