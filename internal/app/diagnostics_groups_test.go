package app

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/application"
	"sshc/internal/diagnostics"
	"sshc/internal/effective"
)

// グループの共通設定は、groups file にメンバーの alias を並べた Host 行として書かれる。
// engine の Diagnostics は、その行を alias の 2 つ目の宣言として数えない。メンバーを
// 踏み台にした経路でも、そのホップを特定できない接続先として扱わない。値の出所には
// 今までどおり並べ、利用者が書いた本当の重複は今までどおり知らせる。
func TestTheEngineDiagnosticsDoNotCountGroupSettingsAsADuplicateAlias(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	files := map[string]string{
		// web-01 は、利用者がグループのディレクトリの外にもう一度書いた本当の重複である。
		"config": "Host app\n\tHostName 203.0.113.40\n\tProxyJump db-primary\n\n" +
			"Host web-01\n\tUser someone-else\n",
		"connections/production/db.conf":  "Host db-primary\n\tHostName 203.0.113.20\n",
		"connections/production/web.conf": "Host web-01\n\tHostName 203.0.113.30\n",
	}
	for name, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	// engine は metadata の巻き戻し用の控えを Vault の鍵で封じるので、グループの設定を
	// 書くには Vault が要る。
	if err := services.vault.Initialise("group settings test passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := services.config.Save(application.EditRequest{
		Kind: application.EditGroups,
		Groups: []application.GroupMetadata{{
			Name: "production", Settings: []application.Setting{{Keyword: "User", Values: []string{"ops"}}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	// 期待するパスは、engine が解決したルートで組み立てる。macOS の一時ディレクトリは
	// /var から /private/var へのリンクを通り、Windows では短い名前（RUNNER~1）で渡る。
	entryFile := services.diagnostics.ConfigPath()
	groupsFile := filepath.Join(filepath.Dir(entryFile), application.DefaultGroupsFile)

	member := inspect(t, services.diagnostics, "db-primary")
	if duplicates := duplicateAliasNotes(member.Projection.Complexities); len(duplicates) != 0 {
		t.Errorf("db-primary complexities = %#v, want no duplicate_alias", duplicates)
	}
	if user := winningSource(member.Projection, "User"); user.Value != "ops" || user.Path != groupsFile {
		t.Errorf("db-primary user source = %#v, want ops from %s", user, groupsFile)
	}

	route := inspect(t, services.diagnostics, "app").Route
	if len(route) != 1 || route[0].Complex || route[0].User != "ops" || route[0].Hostname != "203.0.113.20" {
		t.Errorf("app route = %#v, want db-primary as ops@203.0.113.20", route)
	}

	duplicated := duplicateAliasNotes(inspect(t, services.diagnostics, "web-01").Projection.Complexities)
	if len(duplicated) != 1 || duplicated[0].Path != entryFile {
		t.Errorf("web-01 duplicate_alias = %#v, want the block the user wrote in config", duplicated)
	}
}

func inspect(t *testing.T, service *diagnostics.Service, alias string) diagnostics.Inspection {
	t.Helper()
	inspection, err := service.Inspect(alias)
	if err != nil {
		t.Fatal(err)
	}
	return inspection
}

func winningSource(projection effective.Projection, keyword string) effective.Source {
	for _, source := range projection.Sources {
		if source.Keyword == keyword && source.Winner {
			return source
		}
	}
	return effective.Source{}
}

func duplicateAliasNotes(complexities []effective.Complexity) []effective.Complexity {
	var duplicates []effective.Complexity
	for _, complexity := range complexities {
		if complexity.Code == effective.ComplexityDuplicateAlias {
			duplicates = append(duplicates, complexity)
		}
	}
	return duplicates
}
