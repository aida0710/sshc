package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/diagnostics"
	"sshc/internal/remotekey"
	"sshc/internal/session"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
)

const remoteKeyLine = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPr0nHGmQb99GXmUofxJM4BXGwGzO0jGsQFBspODbkvS fixture@example"

// remoteCall は、リモートで走らせた 1 本のコマンドである。
type remoteCall struct {
	target  sshclient.Target
	command string
	stdin   []byte
}

// sequencedRunner は呼び出しごとに用意した出力を 1 つずつ再生する。
// これにより、ネットワークへ出ずに probe と registration に別々の応答をさせられる。
type sequencedRunner struct {
	commands  []remoteCall
	outputs   []sshclient.Output
	resolved  []string
	beforeRun func(remoteCall)
}

func (runner *sequencedRunner) run(
	_ context.Context, target sshclient.Target, command string, stdin []byte,
) (sshclient.Output, error) {
	call := remoteCall{target: target, command: command, stdin: stdin}
	if runner.beforeRun != nil {
		runner.beforeRun(call)
	}
	runner.commands = append(runner.commands, call)
	if len(runner.outputs) == 0 {
		return sshclient.Output{}, nil
	}
	next := runner.outputs[0]
	runner.outputs = runner.outputs[1:]
	return next, nil
}

func newRemoteKeyServer(t *testing.T, outputs []sshclient.Output) (*echo.Echo, session.Credentials, *sequencedRunner, string) {
	t.Helper()

	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte(diagnosticsConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}

	diagnosticsService := diagnostics.NewService(workspace, (&recordingProbe{}).dial)
	diagnosticsService.Reachability = diagnostics.Reachability{
		Dialer: dialerStub(func(context.Context, string, string) (net.Conn, error) {
			return nil, net.UnknownNetworkError("unreachable in test")
		}),
	}

	runner := &sequencedRunner{outputs: outputs}
	remote := &remotekey.Service{
		// **行き先は登録一回につき一度だけ決まる。** 実際の解決は
		// internal/app が配線するので、ここでは設定を読んで宛先を返す。
		Resolve: func(alias string) (sshclient.Target, error) {
			runner.resolved = append(runner.resolved, alias)
			hostname, port, err := diagnosticsService.Destination(alias)
			if err != nil {
				return sshclient.Target{}, err
			}
			return sshclient.Target{Alias: alias, HostName: hostname, Port: port}, nil
		},
		Run: runner.run,
	}

	sessions, bootstrap, err := session.NewManager(bytes.NewReader(bytes.Repeat([]byte{0x77}, 8192)))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := sessions.Bootstrap(bootstrap)
	if err != nil {
		t.Fatal(err)
	}

	engine := echo.New()
	engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: sessions, Unlocked: alwaysUnlocked}).Middleware)
	registry := actionRegistry{}
	addDiagnosticsActions(registry, diagnosticsService)
	actions := ActionHandlers{Sessions: sessions, Kinds: registry}
	registerActionRoutes(engine, actions)
	registerRemoteKeyRoutes(engine, RemoteKeyHandlers{
		Service:     remote,
		Diagnostics: diagnosticsService,
		Actions:     actions,
	})
	return engine, credentials, runner, diagnosticsService.ConfigPath()
}

func remoteKeyPlanToken(t *testing.T, engine *echo.Echo, credentials session.Credentials, request api.RemoteKeyPlanRequest) string {
	t.Helper()
	response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/plan",
		mustMarshal(t, request), "")
	if response.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", response.Code, response.Body.String())
	}
	var plan api.RemoteKeyPlan
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.ActionToken == "" || plan.ActionExpiresAt == "" {
		t.Fatalf("plan confirmation = %#v", plan)
	}
	return plan.ActionToken
}

func TestRemoteKeyPlanDescribesTheChangeWithoutContactingAnything(t *testing.T) {
	engine, credentials, runner, _ := newRemoteKeyServer(t, nil)

	response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/plan",
		mustMarshal(t, api.RemoteKeyPlanRequest{
			Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine,
		}), "")
	if response.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", response.Code, response.Body.String())
	}
	var payload api.RemoteKeyPlan
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RemotePath != "~/.ssh/authorized_keys" || !payload.Supported {
		t.Fatalf("plan = %#v", payload)
	}
	if payload.Routine != remotekey.Routine {
		t.Error("the plan must show the exact routine that will run")
	}
	if strings.Contains(payload.Routine, "fixture@example") || strings.Contains(payload.Routine, "bastion") {
		t.Fatal("the routine carried caller input")
	}
	if len(payload.Manual) == 0 {
		t.Error("the plan must offer manual instructions")
	}
	// plan は設定が持つ ProxyCommand を表示する。鍵を登録するために
	// 接続すれば、それが実行されてしまうからだ。
	if len(payload.ExecutableDirectives) != 1 {
		t.Errorf("executable directives = %#v", payload.ExecutableDirectives)
	}
	if payload.ActionToken == "" || payload.ActionExpiresAt == "" {
		t.Fatal("the displayed plan must carry its own confirmation")
	}
	if len(runner.commands) != 0 {
		t.Fatal("planning started a process")
	}
}

func TestRemoteKeyRegisterNeedsAConfirmationAndSendsTheKeyOnStdin(t *testing.T) {
	engine, credentials, runner, _ := newRemoteKeyServer(t, []sshclient.Output{
		{Stdout: []byte(remotekey.ProbeMarker + "\n")},
		{Stdout: []byte("sshc: added\n")},
	})
	body := mustMarshal(t, api.RemoteKeyRegisterRequest{
		Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine, AcknowledgeExecutable: true,
	})

	refused := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register", body, "")
	if refused.Code != http.StatusForbidden {
		t.Fatalf("register without a confirmation = %d, want 403", refused.Code)
	}
	if len(runner.commands) != 0 {
		t.Fatal("an unconfirmed registration started a process")
	}

	token := remoteKeyPlanToken(t, engine, credentials, api.RemoteKeyPlanRequest{
		Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine,
	})
	accepted := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register", body, token)
	if accepted.Code != http.StatusOK {
		t.Fatalf("register = %d: %s", accepted.Code, accepted.Body.String())
	}
	var payload api.RemoteKeyRegisterResponse
	if err := json.Unmarshal(accepted.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Outcome != remotekey.RegistrationAdded {
		t.Fatalf("payload = %#v", payload)
	}

	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	register := runner.commands[1]
	if string(register.stdin) != remoteKeyLine+"\n" {
		t.Errorf("stdin = %q, want the key line", register.stdin)
	}
	// コマンドに可変のものが現れてはならない。routine は定数であり、
	// 鍵は標準入力に乗せて運ばれる。
	if register.command != remotekey.Routine {
		t.Fatalf("command = %q, want the fixed routine", register.command)
	}
	if strings.Contains(register.command, "AAAAC3Nza") || strings.Contains(register.command, "fixture@example") {
		t.Fatalf("the command carried key material: %q", register.command)
	}
}

func TestRemoteKeyRegisterRefusesAnUnacknowledgedExecutableDirective(t *testing.T) {
	engine, credentials, runner, _ := newRemoteKeyServer(t, []sshclient.Output{
		{Stdout: []byte(remotekey.ProbeMarker + "\n")},
		{Stdout: []byte("sshc: added\n")},
	})

	token := remoteKeyPlanToken(t, engine, credentials, api.RemoteKeyPlanRequest{
		Alias: "bastion", KeyPath: "x", PublicKey: remoteKeyLine,
	})
	response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register",
		mustMarshal(t, api.RemoteKeyRegisterRequest{
			Alias: "bastion", KeyPath: "x", PublicKey: remoteKeyLine, AcknowledgeExecutable: false,
		}), token)
	if response.Code != http.StatusConflict {
		t.Fatalf("unacknowledged directive = %d, want 409: %s", response.Code, response.Body.String())
	}
	if len(runner.commands) != 0 {
		t.Fatal("a refused registration started a process")
	}
}

func TestRemoteKeyRegisterRejectsAKeyItCannotParse(t *testing.T) {
	engine, credentials, runner, _ := newRemoteKeyServer(t, nil)

	response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register",
		mustMarshal(t, api.RemoteKeyRegisterRequest{
			Alias: "bastion", KeyPath: "x", PublicKey: "rm -rf / AAAA", AcknowledgeExecutable: true,
		}), "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid key = %d, want 400: %s", response.Code, response.Body.String())
	}
	if len(runner.commands) != 0 {
		t.Fatal("an invalid key started a process")
	}
}

func TestRemoteKeyConfirmationCannotBeMintedWithoutDisplayingAPlan(t *testing.T) {
	engine, credentials, runner, _ := newRemoteKeyServer(t, nil)
	response := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/actions",
		mustMarshal(t, api.IssueActionRequest{Kind: session.ActionRemoteKeyRegister, Target: "bastion"}), "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("generic remote-key confirmation = %d, want 400: %s", response.Code, response.Body.String())
	}
	if code := problemCode(t, response.Body.Bytes()); code != "unknown_action_kind" {
		t.Fatalf("problem code = %q, want unknown_action_kind", code)
	}
	if len(runner.commands) != 0 {
		t.Fatal("minting a confirmation started a process")
	}
}

func TestRemoteKeyRegisterRejectsAPlanAfterItsDestinationChanges(t *testing.T) {
	engine, credentials, runner, configPath := newRemoteKeyServer(t, nil)
	request := api.RemoteKeyPlanRequest{
		Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine,
	}

	described := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/plan",
		mustMarshal(t, request), "")
	if described.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", described.Code, described.Body.String())
	}
	var plan struct {
		ActionToken string `json:"actionToken"`
	}
	if err := json.Unmarshal(described.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.ActionToken == "" {
		t.Fatal("plan did not carry its server-derived confirmation")
	}

	changed := strings.ReplaceAll(diagnosticsConfig, "203.0.113.10", "203.0.113.99")
	changed = strings.ReplaceAll(changed, "User ops", "User deploy")
	if err := os.WriteFile(configPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}

	registered := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register",
		mustMarshal(t, api.RemoteKeyRegisterRequest{
			Alias: request.Alias, KeyPath: request.KeyPath, PublicKey: request.PublicKey,
			AcknowledgeExecutable: true,
		}), plan.ActionToken)
	if registered.Code != http.StatusForbidden {
		t.Fatalf("register after destination change = %d, want 403: %s", registered.Code, registered.Body.String())
	}
	if code := problemCode(t, registered.Body.Bytes()); code != "action_token_invalid" {
		t.Fatalf("problem code = %q, want action_token_invalid", code)
	}
	if len(runner.commands) != 0 {
		t.Fatal("a registration with a stale plan started a process")
	}
}

func TestRemoteKeyRegisterRejectsAPlanAfterItsExecutionConfigChanges(t *testing.T) {
	engine, credentials, runner, configPath := newRemoteKeyServer(t, nil)
	request := api.RemoteKeyPlanRequest{
		Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine,
	}
	initial := strings.Replace(diagnosticsConfig, "\tPort 2222\n", "\tPort 2222\n\tIdentityFile id_one\n", 1)
	if err := os.WriteFile(configPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	token := remoteKeyPlanToken(t, engine, credentials, request)

	changed := strings.Replace(initial, "IdentityFile id_one", "IdentityFile id_other", 1)
	if err := os.WriteFile(configPath, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	registered := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register",
		mustMarshal(t, api.RemoteKeyRegisterRequest{
			Alias: request.Alias, KeyPath: request.KeyPath, PublicKey: request.PublicKey,
			AcknowledgeExecutable: true,
		}), token)
	if registered.Code != http.StatusForbidden {
		t.Fatalf("register after config change = %d, want 403: %s", registered.Code, registered.Body.String())
	}
	if code := problemCode(t, registered.Body.Bytes()); code != "action_token_invalid" {
		t.Fatalf("problem code = %q, want action_token_invalid", code)
	}
	if len(runner.commands) != 0 {
		t.Fatal("a registration with a stale execution config started a process")
	}
}

// **行き先は登録一回につき一度だけ決まる。**
//
// かつては設定を凍結してファイルへ書き、`ssh -F` にそれを読ませることで同じ
// 性質を守っていた。プロセス内では読むファイルが無いので、代わりに解決を
// 一度に限る。probe と routine の間に設定を書き換えた者が、二本目の行き先を
// 変えられてはならない。
func TestRemoteKeyRegisterResolvesTheDestinationOnce(t *testing.T) {
	engine, credentials, runner, configPath := newRemoteKeyServer(t, []sshclient.Output{
		{Stdout: []byte(remotekey.ProbeMarker + "\n")},
		{Stdout: []byte("sshc: added\n")},
	})
	request := api.RemoteKeyPlanRequest{
		Alias: "bastion", KeyPath: "~/.ssh/id_ed25519.pub", PublicKey: remoteKeyLine,
	}
	token := remoteKeyPlanToken(t, engine, credentials, request)

	changed := false
	runner.beforeRun = func(remoteCall) {
		if changed {
			return
		}
		changed = true
		mutated := strings.ReplaceAll(diagnosticsConfig, "203.0.113.10", "203.0.113.99")
		if err := os.WriteFile(configPath, []byte(mutated), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	registered := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/remote-keys/register",
		mustMarshal(t, api.RemoteKeyRegisterRequest{
			Alias: request.Alias, KeyPath: request.KeyPath, PublicKey: request.PublicKey,
			AcknowledgeExecutable: true,
		}), token)
	if registered.Code != http.StatusOK {
		t.Fatalf("register = %d: %s", registered.Code, registered.Body.String())
	}
	if len(runner.resolved) != 1 {
		t.Fatalf("the destination was resolved %d times, want once", len(runner.resolved))
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %#v", runner.commands)
	}
	for _, call := range runner.commands {
		if call.target.HostName != "203.0.113.10" {
			t.Fatalf("a command was redirected to %q by an edit made mid-flight", call.target.HostName)
		}
	}
}
