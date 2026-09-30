package app

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/snippets"
)

// startupConfig は、起動スニペットを割り当てる web と、踏み台の bastion を持つ設定である。
const startupConfig = "Host bastion\n\tHostName 198.51.100.1\n\nHost web\n\tHostName 203.0.113.10\n\tUser deploy\n"

// writeSSHConfig は、このワークスペースの ~/.ssh/config を contents で置き換える。
func writeSSHConfig(t *testing.T, services *engineServices, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(services.workspace.Root(), "config"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

// 起動スニペットの割り当ては、engine が接続に使う resolver の binding に結び付く。
// resolver が binding を渡さなければ、どの接続でも「接続先が変わった」になり、本番で
// 起動スニペットが一切送られない。偽の resolver を使うテストでは、それに気付けない。
func TestAStartupSnippetIsBoundToTheDestinationTheEngineConnectsTo(t *testing.T) {
	services, err := newEngineServices(Dependencies{Home: t.TempDir(), Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.vault.Initialise(""); err != nil {
		t.Fatal(err)
	}
	defer services.vault.Lock()
	writeSSHConfig(t, services, startupConfig)
	snippet, err := services.snippets.Create(snippets.Draft{Name: "app", Command: "cd /srv/app"})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.snippets.SetStartup("web", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}

	prepared, err := services.snippets.PrepareStartupCommand("web")
	if err != nil || prepared.Command != "cd /srv/app" {
		t.Fatalf("PrepareStartupCommand = %#v, %v; want the assigned command", prepared, err)
	}

	for name, changed := range map[string]string{
		"HostName":  "Host bastion\n\tHostName 198.51.100.1\n\nHost web\n\tHostName 203.0.113.20\n\tUser deploy\n",
		"User":      "Host bastion\n\tHostName 198.51.100.1\n\nHost web\n\tHostName 203.0.113.10\n\tUser root\n",
		"Port":      "Host bastion\n\tHostName 198.51.100.1\n\nHost web\n\tHostName 203.0.113.10\n\tUser deploy\n\tPort 2222\n",
		"ProxyJump": "Host bastion\n\tHostName 198.51.100.1\n\nHost web\n\tHostName 203.0.113.10\n\tUser deploy\n\tProxyJump bastion\n",
	} {
		t.Run(name, func(t *testing.T) {
			writeSSHConfig(t, services, changed)
			if _, err := services.snippets.PrepareStartupCommand("web"); !errors.Is(err, snippets.ErrStartupDestinationChanged) {
				t.Fatalf("after changing %s, PrepareStartupCommand = %v, want ErrStartupDestinationChanged", name, err)
			}

			writeSSHConfig(t, services, startupConfig)
			if _, err := services.snippets.PrepareStartupCommand("web"); err != nil {
				t.Fatalf("back on the assigned destination, PrepareStartupCommand = %v", err)
			}
		})
	}
}

// ロック中はスニペットの文書を読めない。接続のたびに起動スニペットを確かめる側は、この
// 失敗を secret.ErrLocked として見分け、割り当ての無いホストでも警告を出さないようにする。
func TestALockedVaultReportsTheStartupSnippetLibraryAsLocked(t *testing.T) {
	services, err := newEngineServices(Dependencies{Home: t.TempDir(), Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.vault.Initialise("1234"); err != nil {
		t.Fatal(err)
	}
	writeSSHConfig(t, services, startupConfig)
	snippet, err := services.snippets.Create(snippets.Draft{Name: "app", Command: "cd /srv/app"})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.snippets.SetStartup("web", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}

	services.vault.Lock()

	if _, err := services.snippets.PrepareStartupCommand("web"); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("PrepareStartupCommand while locked = %v, want secret.ErrLocked", err)
	}
}

// Vault を開いたまま接続先を改名すると、起動スニペットの割り当ても新しい alias へ
// 移る。移すのは newEngineServices が配線する StartupRenamer なので、偽の部品を組む
// テストでは、配線が外れても気付けない。
func TestRenamingAHostInTheEngineCarriesItsStartupSnippet(t *testing.T) {
	services, err := newEngineServices(Dependencies{Home: t.TempDir(), Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.vault.Initialise(""); err != nil {
		t.Fatal(err)
	}
	defer services.vault.Lock()
	writeSSHConfig(t, services, startupConfig)
	snippet, err := services.snippets.Create(snippets.Draft{Name: "app", Command: "cd /srv/app"})
	if err != nil {
		t.Fatal(err)
	}
	if err := services.snippets.SetStartup("web", snippet.ID, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := services.config.SaveWithSecrets(application.EditRequest{
		Kind: application.EditRename, Path: "config", Base: startupConfig, Alias: "web", NewAlias: "site",
	}); err != nil {
		t.Fatalf("rename = %v", err)
	}

	assignments, err := services.snippets.Startup()
	if err != nil {
		t.Fatal(err)
	}
	want := []snippets.StartupAssignment{{Alias: "site", SnippetID: snippet.ID}}
	if len(assignments) != 1 || assignments[0] != want[0] {
		t.Fatalf("startup assignments after rename = %#v, want %#v", assignments, want)
	}
	prepared, err := services.snippets.PrepareStartupCommand("site")
	if err != nil || prepared.Command != "cd /srv/app" {
		t.Fatalf("PrepareStartupCommand(site) = %#v, %v; want the command assigned to web", prepared, err)
	}
}
