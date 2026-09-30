package app

import (
	"crypto/rand"
	"errors"
	"testing"

	"sshc/internal/application"
	"sshc/internal/secret/secrettest"
	"sshc/internal/snippets"
	"sshc/internal/vpn"
)

// engineServicesWithVPNProfiles は、Vault のロックを解除し、startupConfig と、VPN
// プロファイル lab と office を持つ engine の部品一式を組む。
func engineServicesWithVPNProfiles(t *testing.T) *engineServices {
	t.Helper()
	// VPN の経路を起動しない。docker を見つけられない環境にして、万一のときもこの機械の
	// docker に触れないようにする。
	services, err := newEngineServices(Dependencies{
		Home: t.TempDir(), Random: rand.Reader, DockerEnvironment: environmentWithoutDocker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.vault.Initialise(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(services.vault.Lock)
	writeSSHConfig(t, services, startupConfig)
	for _, name := range []string{"lab", "office"} {
		change, err := services.config.PlanVPNProfileCreate(application.VPNProfile{
			Name: name, Backend: vpn.WireGuard,
			WireGuard: &application.WireGuardProfile{Servers: []string{name + ".vpn.example.jp"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := services.config.CommitVPNProfileChange(change, nil); err != nil {
			t.Fatal(err)
		}
	}
	return services
}

func attachVPN(t *testing.T, services *engineServices, alias, profile string) {
	t.Helper()
	if _, err := services.config.SetConnectionVPN(alias, profile); err != nil {
		t.Fatal(err)
	}
}

// 画面と CLI が割り当てに記録する値（application の PasswordBinding）と、接続が照合する
// 値（sshParts.target の AuthenticationBinding）は、同じ VPN プロファイルを含む。そのため
// 保存済みのパスワードは、割り当てたときと同じプロファイルを通る接続にだけ渡る。
func TestASavedPasswordIsReleasedOnlyThroughTheVPNProfileItWasAssignedFor(t *testing.T) {
	services := engineServicesWithVPNProfiles(t)
	attachVPN(t, services, "web", "lab")
	binding, err := services.config.PasswordBinding("web")
	if err != nil {
		t.Fatal(err)
	}
	if err := secrettest.StoreDedicatedPassword(services.vault, services.transactions, secrettest.DedicatedPassword{
		Alias: "web", Password: "lab-only", Binding: binding,
	}); err != nil {
		t.Fatal(err)
	}
	password := storedPassword(services.vault)
	releasedOn := func(profile string) string {
		t.Helper()
		attachVPN(t, services, "web", profile)
		target, err := services.ssh.target("web")
		if err != nil {
			t.Fatal(err)
		}
		released, _ := password(target)
		return released
	}

	if got := releasedOn("lab"); got != "lab-only" {
		t.Fatalf("through lab, the saved password = %q, want lab-only", got)
	}
	for _, other := range []string{"office", ""} {
		if got := releasedOn(other); got != "" {
			t.Fatalf("through %q, the password saved for lab was released: %q", other, got)
		}
	}
}

// 起動スニペットの割り当ても、割り当てたときの VPN プロファイルに結び付く。別の
// プロファイルへ付け替えたり外したりした接続には送らない。
func TestAStartupSnippetStopsWhenTheConnectionMovesToAnotherVPNProfile(t *testing.T) {
	services := engineServicesWithVPNProfiles(t)
	attachVPN(t, services, "web", "lab")
	snippet, err := services.snippets.Create(snippets.Draft{Name: "app", Command: "cd /srv/app"})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.snippets.SetStartup("web", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}

	for _, other := range []string{"office", ""} {
		attachVPN(t, services, "web", other)
		if _, err := services.snippets.PrepareStartupCommand("web"); !errors.Is(err, snippets.ErrStartupDestinationChanged) {
			t.Fatalf("through %q, PrepareStartupCommand = %v, want ErrStartupDestinationChanged", other, err)
		}
	}
	attachVPN(t, services, "web", "lab")
	if prepared, err := services.snippets.PrepareStartupCommand("web"); err != nil || prepared.Command != "cd /srv/app" {
		t.Fatalf("back through lab, PrepareStartupCommand = %#v, %v; want the assigned command", prepared, err)
	}
}

