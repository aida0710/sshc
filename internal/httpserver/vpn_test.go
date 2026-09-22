package httpserver

import (
	"crypto/rand"
	"encoding/json"
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
	registerVPNRoutes(engine, VPNHandlers{
		Config: config, Secrets: secrets,
		Sessions: vpn.New(filepath.Join(root, "sshc", "vpn"), os.Getuid()),
	})
	return engine, secrets, config
}

func labProfileBody(withSecret bool) string {
	body := `{"profile":{"name":"lab","backend":"wireguard","target":"10.9.9.1:22",` +
		`"wireguard":{"server":"vpn.example.jp:51820","peerPublicKey":"` + testVPNPublicKey + `",` +
		`"address":"10.9.9.2/32"}}`
	if withSecret {
		body += `,"secrets":{"wireguardPrivateKey":"` + testVPNPrivateKey + `"}`
	}
	return body + "}"
}

func decodeOverview(t *testing.T, payload []byte) vpnOverviewResponse {
	t.Helper()
	var overview vpnOverviewResponse
	if err := json.Unmarshal(payload, &overview); err != nil {
		t.Fatalf("overview = %s: %v", payload, err)
	}
	return overview
}

// プロファイルを保存し、接続へ結び付け、一覧に現れる。
func TestASavedProfileIsListedWithTheConnectionsThatUseIt(t *testing.T) {
	engine, _, config := vpnEngine(t)

	saved := send(t, engine, http.MethodPut, "/api/v1/vpn/profiles/lab", labProfileBody(true), nil)
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
	if session.Profile.Name != "lab" || session.Profile.Target != "10.9.9.1:22" {
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
	if body := send(t, engine, http.MethodPut, "/api/v1/vpn/profiles/lab", labProfileBody(true), nil); body.Code != http.StatusOK {
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
	send(t, engine, http.MethodPut, "/api/v1/vpn/profiles/lab", labProfileBody(true), nil)
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
	body := strings.Replace(labProfileBody(true), "10.9.9.1:22", "lab.example.jp:22", 1)

	refused := send(t, engine, http.MethodPut, "/api/v1/vpn/profiles/lab", body, nil)

	if refused.Code != http.StatusBadRequest || !strings.Contains(refused.Body.String(), "vpn_profile_invalid") {
		t.Fatalf("save = %d: %s", refused.Code, refused.Body.String())
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
