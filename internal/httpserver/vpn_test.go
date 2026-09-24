package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
)

const (
	testVPNPrivateKey = "aAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAaAA="
	testVPNPublicKey  = "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA="
	vaultPassphrase   = "a vpn handler test passphrase"
)

// vpnEngine は、VPN の経路を扱う engine を一台組む。Docker は使わない。
// ここで確かめるのは、設定と秘密の扱いと、応答の形だからである。
func vpnEngine(t *testing.T) (*echo.Echo, *secret.Service, *application.Service) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Host lab\n  HostName 10.9.9.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	transactions := storage.NewManager(workspace, time.Now, rand.Reader)
	config := application.NewService(workspace, transactions)
	secrets := secret.NewService(workspace, transactions, time.Now)
	if err := secrets.Initialise(vaultPassphrase); err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	sessions := vpn.New(filepath.Join(root, "sshc", "vpn"), os.Getuid(), nil)
	registerVPNRoutes(engine, VPNHandlers{
		Config: config,
		Profiles: vpnprofile.New(vpnprofile.Dependencies{
			Configuration: config, Vault: secrets, Routes: sessions,
		}),
		Sessions: sessions,
	})
	return engine, secrets, config
}

func decodeProblem(t *testing.T, payload []byte) problemPayload {
	t.Helper()
	var refused problemPayload
	if err := json.Unmarshal(payload, &refused); err != nil {
		t.Fatalf("problem = %s: %v", payload, err)
	}
	return refused
}

func labProfileBody(withSecret bool) string {
	body := `{"profile":{"name":"lab","backend":"wireguard",` +
		`"wireguard":{"server":"vpn.example.jp:51820","peerPublicKey":"` + testVPNPublicKey + `",` +
		`"address":"10.9.9.2/32"}}`
	if withSecret {
		body += `,"secrets":{"wireguardPrivateKey":"` + testVPNPrivateKey + `"}`
	}
	return body + "}"
}

func decodeOverview(t *testing.T, payload []byte) VPNOverview {
	t.Helper()
	var overview VPNOverview
	if err := json.Unmarshal(payload, &overview); err != nil {
		t.Fatalf("overview = %s: %v", payload, err)
	}
	return overview
}

// プロファイルを保存し、接続へ結び付け、一覧に現れる。
func TestASavedProfileIsListedWithTheConnectionsThatUseIt(t *testing.T) {
	engine, _, config := vpnEngine(t)

	saved := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", saved.Code, saved.Body.String())
	}
	bound := send(t, engine, http.MethodPut, "/api/v1/vpn/bindings", `{"alias":"lab","profile":"lab"}`, nil)
	if bound.Code != http.StatusOK {
		t.Fatalf("bind = %d: %s", bound.Code, bound.Body.String())
	}

	overview := decodeOverview(t, bound.Body.Bytes())
	if len(overview.Profiles) != 1 {
		t.Fatalf("profiles = %+v", overview.Profiles)
	}
	session := overview.Profiles[0]
	if session.Profile.Name != "lab" || session.Profile.Backend != "wireguard" {
		t.Fatalf("profile = %+v", session.Profile)
	}
	if len(session.Connections) != 1 || session.Connections[0] != "lab" {
		t.Fatalf("connections = %v", session.Connections)
	}
	name, err := config.ConnectionVPN("lab")
	if err != nil || name != "lab" {
		t.Fatalf("ConnectionVPN = %q, %v", name, err)
	}
}

// 応答にもログにも秘密鍵は現れない。
func TestNoVPNRouteEverReturnsTheStoredSecret(t *testing.T) {
	engine, secrets, _ := vpnEngine(t)
	if body := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil); body.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", body.Code, body.Body.String())
	}

	for _, route := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/vpn", ""},
		{http.MethodPut, "/api/v1/vpn/profiles/lab", labProfileBody(false)},
		{http.MethodPut, "/api/v1/vpn/bindings", `{"alias":"lab","profile":"lab"}`},
	} {
		answer := send(t, engine, route.method, route.path, route.body, nil)
		if strings.Contains(answer.Body.String(), testVPNPrivateKey) {
			t.Fatalf("%s %s returned the private key: %s", route.method, route.path, answer.Body.String())
		}
	}

	// 設定だけを保存し直しても、秘密は残る。
	stored, err := secrets.VPNSecrets("lab")
	if err != nil || !strings.Contains(stored, testVPNPrivateKey) {
		t.Fatalf("VPNSecrets = %q, %v", stored, err)
	}
}

// プロファイルを消すと、それを指していた接続の紐付けと秘密も消える。
func TestRemovingAProfileClearsItsBindingsAndSecrets(t *testing.T) {
	engine, secrets, config := vpnEngine(t)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)
	send(t, engine, http.MethodPut, "/api/v1/vpn/bindings", `{"alias":"lab","profile":"lab"}`, nil)

	removed := send(t, engine, http.MethodDelete, "/api/v1/vpn/profiles/lab", "", nil)

	if removed.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", removed.Code, removed.Body.String())
	}
	if overview := decodeOverview(t, removed.Body.Bytes()); len(overview.Profiles) != 0 {
		t.Fatalf("profiles after delete = %+v", overview.Profiles)
	}
	name, err := config.ConnectionVPN("lab")
	if err != nil || name != "" {
		t.Fatalf("消えたプロファイルを指したままの接続が残った: %q, %v", name, err)
	}
	if _, err := secrets.VPNSecrets("lab"); err == nil {
		t.Fatal("秘密が残った")
	}
}

// 経路を作れない指定は、保存の時点で断る。
func TestAProfileThatCannotBecomeARouteIsRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)
	body := strings.Replace(labProfileBody(true), `"backend":"wireguard",`, `"backend":"wireguard","dns":["dns.example.jp"],`, 1)

	refused := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", body, nil)

	if refused.Code != http.StatusBadRequest {
		t.Fatalf("save = %d: %s", refused.Code, refused.Body.String())
	}
	want := problemPayload{Code: "vpn_profile_invalid", Message: "request rejected", Field: "dns", Reason: "not_ipv4"}
	if got := decodeProblem(t, refused.Body.Bytes()); got.Code != want.Code || got.Field != want.Field || got.Reason != want.Reason {
		t.Fatalf("problem = %+v, want %+v", got, want)
	}
}

// 無いプロファイルへ接続を結び付けない。
func TestBindingToAnUnknownProfileIsRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)

	refused := send(t, engine, http.MethodPut, "/api/v1/vpn/bindings", `{"alias":"lab","profile":"absent"}`, nil)

	if refused.Code != http.StatusNotFound || !strings.Contains(refused.Body.String(), "vpn_profile_unknown") {
		t.Fatalf("bind = %d: %s", refused.Code, refused.Body.String())
	}
}

// 改名すると、設定・秘密・接続の紐付けが揃って新しい名前へ移る。
func TestRenamingAProfileCarriesItsSecretsAndBindings(t *testing.T) {
	engine, secrets, config := vpnEngine(t)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)
	send(t, engine, http.MethodPut, "/api/v1/vpn/bindings", `{"alias":"lab","profile":"lab"}`, nil)

	renamed := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles/lab/rename", `{"name":"tains"}`, nil)

	if renamed.Code != http.StatusOK {
		t.Fatalf("rename = %d: %s", renamed.Code, renamed.Body.String())
	}
	overview := decodeOverview(t, renamed.Body.Bytes())
	if len(overview.Profiles) != 1 || overview.Profiles[0].Profile.Name != "tains" {
		t.Fatalf("profiles = %+v", overview.Profiles)
	}
	if connections := overview.Profiles[0].Connections; len(connections) != 1 || connections[0] != "lab" {
		t.Fatalf("connections = %v", connections)
	}
	name, err := config.ConnectionVPN("lab")
	if err != nil || name != "tains" {
		t.Fatalf("古い名前を指したままの接続が残った: %q, %v", name, err)
	}
	stored, err := secrets.VPNSecrets("tains")
	if err != nil || !strings.Contains(stored, testVPNPrivateKey) {
		t.Fatalf("VPNSecrets(tains) = %q, %v", stored, err)
	}
	if _, err := secrets.VPNSecrets("lab"); err == nil {
		t.Fatal("古い名前の秘密が残った")
	}
}

// すでにある名前へは改名しない。
func TestRenamingOntoAnExistingProfileIsRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles",
		strings.ReplaceAll(labProfileBody(true), `"lab"`, `"office"`), nil)

	refused := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles/lab/rename", `{"name":"office"}`, nil)

	if refused.Code != http.StatusConflict || !strings.Contains(refused.Body.String(), "vpn_profile_exists") {
		t.Fatalf("rename = %d: %s", refused.Code, refused.Body.String())
	}
}

// openconnect のプロファイルも、設定は metadata へ、パスワードは Vault へ入る。
func TestAnOpenConnectProfileKeepsItsPasswordOutOfEveryResponse(t *testing.T) {
	engine, secrets, _ := vpnEngine(t)
	const password = "an openconnect password"
	body := `{"profile":{"name":"office","backend":"openconnect",` +
		`"openconnect":{"server":"vpn.example.jp","username":"fixture","protocol":"anyconnect"}},` +
		`"secrets":{"openconnectPassword":"` + password + `"}}`

	saved := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", body, nil)

	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", saved.Code, saved.Body.String())
	}
	if strings.Contains(saved.Body.String(), password) {
		t.Fatalf("応答にパスワードが現れた: %s", saved.Body.String())
	}
	overview := decodeOverview(t, saved.Body.Bytes())
	if len(overview.Profiles) != 1 || overview.Profiles[0].Profile.OpenConnect == nil ||
		overview.Profiles[0].Profile.OpenConnect.Username != "fixture" {
		t.Fatalf("profiles = %+v", overview.Profiles)
	}
	stored, err := secrets.VPNSecrets("office")
	if err != nil || !strings.Contains(stored, password) {
		t.Fatalf("VPNSecrets = %q, %v", stored, err)
	}
}

// 二段目のTOTPの種も、応答には現れない。
func TestTheSecondFactorSeedNeverLeavesTheVault(t *testing.T) {
	engine, secrets, _ := vpnEngine(t)
	const seed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	body := `{"profile":{"name":"office","backend":"openconnect",` +
		`"openconnect":{"server":"vpn.example.jp","username":"fixture","secondFactor":"totp"}},` +
		`"secrets":{"openconnectPassword":"a password","openconnectTotpSecret":"` + seed + `"}}`

	saved := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", body, nil)
	listed := send(t, engine, http.MethodGet, "/api/v1/vpn", "", nil)

	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", saved.Code, saved.Body.String())
	}
	for _, answer := range []string{saved.Body.String(), listed.Body.String()} {
		if strings.Contains(answer, seed) {
			t.Fatalf("応答に二段目の種が現れた: %s", answer)
		}
	}
	stored, err := secrets.VPNSecrets("office")
	if err != nil || !strings.Contains(stored, seed) {
		t.Fatalf("VPNSecrets = %q, %v", stored, err)
	}
}

// 無いプロファイルのログは無い。
func TestLogsForAnUnknownProfileAreRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)

	refused := send(t, engine, http.MethodGet, "/api/v1/vpn/profiles/absent/logs", "", nil)

	if refused.Code != http.StatusNotFound || !strings.Contains(refused.Body.String(), "vpn_profile_unknown") {
		t.Fatalf("logs = %d: %s", refused.Code, refused.Body.String())
	}
}

// 同じ名前のプロファイルは作らない。上書きは更新の役目である。
func TestCreatingAProfileTwiceIsRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)

	refused := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)

	if refused.Code != http.StatusConflict || decodeProblem(t, refused.Body.Bytes()).Code != "vpn_profile_exists" {
		t.Fatalf("create twice = %d: %s", refused.Code, refused.Body.String())
	}
}

// 無いプロファイルは更新しない。作成は作成の役目である。
func TestUpdatingAnUnknownProfileIsRefused(t *testing.T) {
	engine, _, _ := vpnEngine(t)

	refused := send(t, engine, http.MethodPut, "/api/v1/vpn/profiles/lab", labProfileBody(true), nil)

	if refused.Code != http.StatusNotFound || decodeProblem(t, refused.Body.Bytes()).Code != "vpn_profile_unknown" {
		t.Fatalf("update = %d: %s", refused.Code, refused.Body.String())
	}
}

// 項目の誤りは、項目の JSON パスと理由の語と上限を返す。
func TestARefusedFieldCarriesItsPathReasonAndLimit(t *testing.T) {
	engine, _, _ := vpnEngine(t)
	body := strings.Replace(labProfileBody(true), `"backend":"wireguard",`,
		`"backend":"wireguard","dns":["10.9.9.53","10.9.9.54","10.9.9.55","10.9.9.56"],`, 1)

	refused := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", body, nil)

	got := decodeProblem(t, refused.Body.Bytes())
	if refused.Code != http.StatusBadRequest || got.Code != "vpn_profile_invalid" ||
		got.Field != "dns" || got.Reason != "too_many" || got.Limit != 3 {
		t.Fatalf("create = %d: %+v", refused.Code, got)
	}
}

// 作成に要る秘密が無ければ、どの秘密が無いかを返し、何も作らない。
func TestCreatingWithoutTheRequiredSecretNamesTheMissingSecret(t *testing.T) {
	engine, _, config := vpnEngine(t)

	refused := send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(false), nil)

	got := decodeProblem(t, refused.Body.Bytes())
	if refused.Code != http.StatusConflict || got.Code != "vpn_secrets_missing" ||
		got.Field != "secrets."+vpn.SecretKeyWireGuardPrivateKey || got.Reason != "required" {
		t.Fatalf("create = %d: %+v", refused.Code, got)
	}
	if profiles, err := config.VPNProfiles(); err != nil || len(profiles) != 0 {
		t.Fatalf("断ったのにプロファイルが残った: %+v, %v", profiles, err)
	}
}

// Vault がロック中なら削除を断り、設定も秘密も残す。
func TestRemovingAProfileWhileTheVaultIsLockedChangesNothing(t *testing.T) {
	engine, secrets, config := vpnEngine(t)
	send(t, engine, http.MethodPost, "/api/v1/vpn/profiles", labProfileBody(true), nil)
	secrets.Lock()

	refused := send(t, engine, http.MethodDelete, "/api/v1/vpn/profiles/lab", "", nil)

	if refused.Code != http.StatusConflict || decodeProblem(t, refused.Body.Bytes()).Code != "vault_locked" {
		t.Fatalf("delete = %d: %s", refused.Code, refused.Body.String())
	}
	if profiles, err := config.VPNProfiles(); err != nil || len(profiles) != 1 {
		t.Fatalf("profiles = %+v, %v", profiles, err)
	}
}

// 対応表で見分ける拒否は、決まったコードと理由の語を返す。
func TestVPNRefusalsCarryTheirCodes(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		want   problemPayload
	}{
		{
			name: "接続先の形", err: &vpn.DestinationError{Address: "db:22", Reason: vpn.ReasonNameNeedsDNS},
			status: http.StatusBadRequest, want: problemPayload{Code: "vpn_destination_invalid", Reason: "name_needs_dns"},
		},
		{
			name: "Dockerが起動していない", err: fmt.Errorf("%w: cannot connect", vpn.ErrDockerNotRunning),
			status: http.StatusConflict, want: problemPayload{Code: "vpn_docker_not_running"},
		},
		{
			name: "ソケットのパスが長すぎる", err: fmt.Errorf("%w: 106 > 103", vpn.ErrSocketPath),
			status: http.StatusConflict, want: problemPayload{Code: "vpn_socket_path_too_long"},
		},
		{
			name: "経路を用意できなかった", err: &vpn.SessionFailure{Profile: "lab", Reason: vpn.FailureHandshakeTimeout},
			status: http.StatusConflict, want: problemPayload{Code: "vpn_session_failed", Reason: "handshake_timeout"},
		},
		{
			name: "同じ名前がある", err: fmt.Errorf("%w: lab", application.ErrVPNProfileExists),
			status: http.StatusConflict, want: problemPayload{Code: "vpn_profile_exists"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := echo.New()
			engine.GET("/refusal", func(c *echo.Context) error { return vpnProblem(c, test.err) })

			answer := send(t, engine, http.MethodGet, "/refusal", "", nil)

			got := decodeProblem(t, answer.Body.Bytes())
			if answer.Code != test.status || got.Code != test.want.Code || got.Reason != test.want.Reason {
				t.Fatalf("problem = %d %+v, want %d %+v", answer.Code, got, test.status, test.want)
			}
		})
	}
}
