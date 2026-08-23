package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/secret"
)

// README の操作説明が現在の CLI と一致することを検証する。

func repositoryFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(contents)
}

// 削除済みのコマンドを現行手順として案内していないことを検証する。
func TestNoDocumentationTeachesTheRemovedEntryPoints(t *testing.T) {
	// docs/superpowers は設計と計画の記録であり、そこには「何を消したか」が
	// 書かれている。歴史を書いた文書から歴史の語を消させない。
	for _, path := range [][]string{{"README.md"}, {"docs", "manual-acceptance.md"}} {
		name := filepath.Join(path...)
		contents, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, path...)...))
		if err != nil {
			continue
		}
		for _, removed := range []string{"--own-engine", "-open=false", "sshc engine start", "make desktop-dist"} {
			if strings.Contains(string(contents), removed) &&
				!strings.Contains(string(contents), removed+"` は未定義") &&
				!strings.Contains(string(contents), "旧版") {
				t.Errorf("%s still teaches %q", name, removed)
			}
		}
	}
}

// engine の起動方法と CLI の役割が明記されていることを検証する。
func TestTheReadmeSaysWhoOwnsTheEngine(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	for phrase, why := range map[string]string{
		"sshc engine": "the way to own an engine from a terminal",
		"sshc vault":  "where passwords are typed",
		"sshc run":    "how a written procedure reaches a host",
	} {
		if !strings.Contains(readme, phrase) {
			t.Errorf("README never mentions %q (%s)", phrase, why)
		}
	}

	if !strings.Contains(readme, "引数なしの `sshc` はエンジンを起動しません") {
		t.Error("README does not say that bare sshc starts no engine")
	}
}

// Vault のタイムアウトと入力経路が実装と一致することを検証する。
func TestTheReadmeStatesTheVaultRules(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	stated := fmt.Sprintf("%d 時間", int(secret.IdleTimeout.Hours()))
	if !strings.Contains(readme, stated) {
		t.Errorf("README does not state the idle timeout %q; internal/secret.IdleTimeout is %v",
			stated, secret.IdleTimeout)
	}
	for _, rule := range []struct {
		text string
		why  string
	}{
		{"Web UI または `sshc vault`", "the supported master-password entry points"},
		{"CLI は対話端末からのみ", "the CLI TTY requirement"},
		{"引数や環境変数", "the inputs rejected by the CLI"},
	} {
		if !strings.Contains(readme, rule.text) {
			t.Errorf("README does not state %s (%q is missing)", rule.why, rule.text)
		}
	}
}

// engine はフォアグラウンドで動作するため、プロセス管理方法を明記する。
func TestTheReadmeSaysHowToKeepTheEngineAlive(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	if !strings.Contains(readme, "自動起動は OS のプロセス管理機能で設定します") {
		t.Error("README does not say autostart belongs to the operating system")
	}
	for _, how := range []string{"tmux", "systemd", "launchd"} {
		if !strings.Contains(readme, how) {
			t.Errorf("README does not say how to keep the engine alive: %q missing", how)
		}
	}
	if !strings.Contains(readme, "デーモン化しません") {
		t.Error("README does not say the engine never detaches")
	}
}
