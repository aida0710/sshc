package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// currentUsageDocuments は、いまのCLIの使い方を案内する文書である。
//
// docs/superpowers、docs/releases、監査の記録は、何を消したか・その時点で何を公開したかを
// 書いた記録であり、ここには入れない。歴史を書いた文書から歴史の語を消させない。
// docs/design.md は「受け付けない書き方」を名指しで書くので、これも入れない。
func currentUsageDocuments(t *testing.T) [][]string {
	t.Helper()
	documents := [][]string{
		{"README.md"},
		{"docs", "manual-acceptance.md"},
		{"docs", "headless-examples.md"},
	}
	pagesRoot := filepath.Join("..", "..", "pages")
	err := filepath.WalkDir(pagesRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".")) && path != pagesRoot {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		relative, err := filepath.Rel(filepath.Join("..", ".."), path)
		if err != nil {
			return err
		}
		documents = append(documents, strings.Split(filepath.ToSlash(relative), "/"))
		return nil
	})
	if err != nil {
		t.Fatalf("walk pages: %v", err)
	}
	return documents
}

// 削除済みのコマンドを現行手順として案内していないことを検証する。
func TestNoDocumentationTeachesTheRemovedEntryPoints(t *testing.T) {
	removedEntryPoints := []string{
		"--own-engine", "-open=false", "sshc engine start", "make desktop-dist",
		// SSH接続はtransportの`ssh`を必ず書く。省略した形は unknown command で断られる。
		"sshc <接続先>", "sshc <alias>", "sshc <host>", "sshc <target>",
	}
	for _, path := range currentUsageDocuments(t) {
		name := filepath.Join(path...)
		contents, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, path...)...))
		if err != nil {
			continue
		}
		for _, taught := range removedEntryPointsTaughtIn(string(contents), removedEntryPoints) {
			t.Errorf("%s still teaches %s", name, taught)
		}
	}
}

// removedEntryPointsTaughtIn は、文書の中で削除済みのコマンドを案内している行を返す。
// 未定義であることや旧バージョンのこととして同じ行で書いたものは除く。除外を文書全体では
// なく行に限るのは、「旧バージョン」をどこかに書いたページのほかの行の案内を見逃さないため。
func removedEntryPointsTaughtIn(contents string, removedEntryPoints []string) []string {
	var taught []string
	lineNumber := 0
	for line := range strings.Lines(contents) {
		lineNumber++
		for _, removed := range removedEntryPoints {
			if !strings.Contains(line, removed) ||
				strings.Contains(line, removed+"` は未定義") ||
				strings.Contains(line, "旧バージョン") {
				continue
			}
			taught = append(taught, fmt.Sprintf("%q on line %d", removed, lineNumber))
		}
	}
	return taught
}

// 「旧バージョン」を書いたページでも、ほかの行で削除済みのコマンドを案内していれば見つける。
func TestARemovedEntryPointIsExcusedOnlyOnTheLineThatCallsItOld(t *testing.T) {
	page := "旧バージョンの`sshc <接続先>`は使えません。\n\n接続するには`sshc <接続先>`を実行します。\n"
	taught := removedEntryPointsTaughtIn(page, []string{"sshc <接続先>"})
	want := []string{`"sshc <接続先>" on line 3`}
	if !slices.Equal(taught, want) {
		t.Errorf("removedEntryPointsTaughtIn = %q, want %q", taught, want)
	}
}

// engine の起動方法と CLI の役割が明記されていることを検証する。
func TestTheReadmeSaysWhoOwnsTheEngine(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	for phrase, why := range map[string]string{
		"sshc engine":          "the way to own an engine from a terminal",
		"sshc vault":           "where passwords are typed",
		"sshc ssh <接続先>":       "how an interactive procedure reaches a host",
		"--non-interactive --": "how a written procedure runs without a terminal",
	} {
		if !strings.Contains(readme, phrase) {
			t.Errorf("README never mentions %q (%s)", phrase, why)
		}
	}

	if !strings.Contains(readme, "引数なしの`sshc`はsshcエンジンを起動しません") {
		t.Error("README does not say that bare sshc starts no engine")
	}
}

// Vault のタイムアウトと入力経路が実装と一致することを検証する。
func TestTheReadmeStatesTheVaultRules(t *testing.T) {
	readme := repositoryFile(t, "README.md")

	stated := fmt.Sprintf("%d時間", int(secret.IdleTimeout.Hours()))
	if !strings.Contains(readme, stated) {
		t.Errorf("README does not state the idle timeout %q; internal/secret.IdleTimeout is %v",
			stated, secret.IdleTimeout)
	}
	for _, rule := range []struct {
		text string
		why  string
	}{
		{"Web UIまたは`sshc vault`", "the supported master-password entry points"},
		{"CLIでは対話ターミナルからの入力だけ", "the CLI TTY requirement"},
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

	if !strings.Contains(readme, "自動起動はOSのプロセス管理機能で設定します") {
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
