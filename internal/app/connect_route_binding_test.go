package app

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/secret/secrettest"
)

// bastionThroughLabConfig は、踏み台 bastion を通って web へ繋ぐ設定である。
const bastionThroughLabConfig = "Host bastion\n\tHostName 198.51.100.1\n\n" +
	"Host web\n\tHostName 203.0.113.10\n\tProxyJump bastion\n"

// savedPasswordsAnswer は、`sshc ssh` が /cli/connect から受け取る保存済みパスワードの欄である。
type savedPasswordsAnswer struct {
	Passwords      map[string]string `json:"passwords"`
	StalePasswords []string          `json:"stalePasswords"`
}

// askToConnect は、`sshc ssh <alias>` と同じく、handoff の秘密を添えて接続に要るものを尋ねる。
func askToConnect(t *testing.T, found handoff.Handoff, alias string) savedPasswordsAnswer {
	t.Helper()
	body, err := json.Marshal(map[string]string{"alias": alias})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, found.URL+httpserver.ConnectPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("connect %s = %d", alias, response.StatusCode)
	}
	var answer savedPasswordsAnswer
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

// VPN を付けた踏み台に保存したパスワードは、その踏み台へ単独で繋ぐ経路（VPN を通る）に
// 結び付く。別の接続の踏み台として通るホップは VPN を通らないので、そのパスワードは渡さず、
// `sshc ssh` には停止中として知らせる。sshcエンジンは、埋め込みターミナルが照合するのと
// 同じホップとしての値で決める。単独の値で決めると、渡したパスワードを CLI が照合で
// 捨て、理由の説明なしに入力を求めることになる。
func TestTheSavedPasswordOfABastionWithAVPNIsNotReleasedAsAHopAndIsReportedAsStopped(t *testing.T) {
	prepared := engineServicesWithVPNProfiles(t)
	writeSSHConfig(t, prepared, bastionThroughLabConfig)
	attachVPN(t, prepared, "bastion", "lab")
	binding, err := prepared.config.PasswordBinding("bastion")
	if err != nil {
		t.Fatal(err)
	}
	if err := secrettest.StoreDedicatedPassword(prepared.vault, prepared.transactions, secrettest.DedicatedPassword{
		Alias: "bastion", Password: "bastion-password", Binding: binding,
	}); err != nil {
		t.Fatal(err)
	}
	dependencies := testDependencies(t)
	dependencies.Home, dependencies.Random = prepared.workspace.Home(), rand.Reader
	built, err := build(dependencies, "test")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- built.server.Serve() }()
	t.Cleanup(func() {
		if err := built.unwind(dependencies); err != nil {
			t.Errorf("unwind = %v", err)
		}
		if err := <-served; err != nil {
			t.Errorf("Serve = %v", err)
		}
	})

	direct := askToConnect(t, built.document, "bastion")
	if direct.Passwords["bastion"] != "bastion-password" {
		t.Fatalf("connecting to bastion itself, passwords = %v; want the saved password", direct.Passwords)
	}
	throughHop := askToConnect(t, built.document, "web")
	if throughHop.Passwords["bastion"] != "" || !slices.Contains(throughHop.StalePasswords, "bastion") {
		t.Fatalf("connecting to web through bastion, answer = %+v; want the password of bastion reported as stopped", throughHop)
	}
}
